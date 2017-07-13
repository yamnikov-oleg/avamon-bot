package telegrambot

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/yamnikov-oleg/avamon-bot/monitor"

	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

var (
	// Green clover
	okStatusEmoji = string([]rune{0x2618, 0xfe0f})
	// Red alarm light
	errorStatusEmoji = string([]rune{0x1f6a8})
)

func replaceHTML(input string) string {
	input = strings.Replace(input, "<", "&lt;", -1)
	input = strings.Replace(input, ">", "&gt;", -1)
	return input
}

type Bot struct {
	AdminNickname string
	DB            *TargetsDB
	TgBot         *tgbotapi.BotAPI
	Monitor       *monitor.Monitor
	sessionMap    map[int64]*session
}

func (b *Bot) formatStatusUpdate(target monitor.Target, status monitor.Status) string {
	var output string
	var sign string

	if status.Type == monitor.StatusOK {
		sign = strings.Repeat(okStatusEmoji, 10) + "\n"
	} else {
		sign = strings.Repeat(errorStatusEmoji, 10) + "\n"
	}

	output += sign
	output += fmt.Sprintf("<b>%v:</b> <b>%v</b>\n\n", replaceHTML(target.Title), status.Type)
	output += fmt.Sprintf("<b>URL:</b> %v\n", replaceHTML(target.URL))
	output += fmt.Sprintf("<b>Response time:</b> %v\n", status.ResponseTime)

	if status.Type != monitor.StatusOK {
		output += fmt.Sprintf("<b>Error msg:</b> %v\n", replaceHTML(status.Err.Error()))
	}
	if status.Type == monitor.StatusHTTPError {
		output += fmt.Sprintf("<b>HTTP Status:</b> %v %v\n", status.HTTPStatusCode, http.StatusText(status.HTTPStatusCode))
	}
	output += sign

	return output
}

func (b *Bot) SendMessage(chatID int64, message string) {
	msg := tgbotapi.NewMessage(chatID, message)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.DisableWebPagePreview = true
	b.TgBot.Send(msg)
}

func (b *Bot) SendDialogMessage(replyTo *tgbotapi.Message, message string) {
	msg := tgbotapi.NewMessage(replyTo.Chat.ID, message)
	msg.ReplyToMessageID = replyTo.MessageID
	msg.ReplyMarkup = tgbotapi.ForceReply{
		ForceReply: true,
		Selective:  true,
	}
	msg.ParseMode = tgbotapi.ModeHTML
	msg.DisableWebPagePreview = true
	b.TgBot.Send(msg)
}

func (b *Bot) MonitorStart() {
	go func() {
		for upd := range b.Monitor.Updates {
			rec, err := b.DB.GetTarget(int(upd.Target.ID))
			if err != nil {
				fmt.Println(err)
				continue
			}
			b.SendMessage(
				rec.ChatID,
				b.formatStatusUpdate(upd.Target, upd.Status))
		}
	}()

	go func() {
		for err := range b.Monitor.Errors() {
			fmt.Println(err)
		}
	}()

	go b.Monitor.Run(nil)
}

type session struct {
	Stage  int
	Dialog dialog
}

type dialog interface {
	ContinueDialog(stepNumber int, update tgbotapi.Update, bot *tgbotapi.BotAPI) (int, bool)
}

type addNewTarget struct {
	Title string
	URL   string
	bot   *Bot
}

func (t *addNewTarget) ContinueDialog(stepNumber int, update tgbotapi.Update, bot *tgbotapi.BotAPI) (int, bool) {
	if stepNumber == 1 {
		t.bot.SendDialogMessage(update.Message, "Enter the title for the target")
		return 2, true
	}
	if stepNumber == 2 {
		t.Title = update.Message.Text
		t.bot.SendDialogMessage(update.Message, "Enter the url for the target")
		return 3, true
	}
	if stepNumber == 3 {
		if _, err := url.Parse(update.Message.Text); err != nil {
			t.bot.SendDialogMessage(update.Message, "Error while parsing url, please try again")
			return 3, true
		}
		t.URL = update.Message.Text
		err := t.bot.DB.CreateTarget(Record{
			ChatID: update.Message.Chat.ID,
			Title:  t.Title,
			URL:    t.URL,
		})
		if err != nil {
			t.bot.SendMessage(
				update.Message.Chat.ID,
				fmt.Sprintf(
					"Error while adding the target, please contact the administrator: %v",
					t.bot.AdminNickname))
			return 0, false
		}
		t.bot.SendMessage(update.Message.Chat.ID, "Target was successfully added")
		return 0, false
	}
	return 0, false
}

type deleteTarget struct {
	bot *Bot
}

func (t *deleteTarget) ContinueDialog(stepNumber int, update tgbotapi.Update, bot *tgbotapi.BotAPI) (int, bool) {
	if stepNumber == 1 {
		targs, err := t.bot.DB.GetCurrentTargets(update.Message.Chat.ID)
		if err != nil {
			t.bot.SendMessage(
				update.Message.Chat.ID,
				fmt.Sprintf(
					"Error while retrieving the targets, please contact the administrator: %v",
					t.bot.AdminNickname))
			return 0, false
		}
		if len(targs) == 0 {
			t.bot.SendMessage(update.Message.Chat.ID, "You have no targets added! Use /add to add one")
			return 0, false
		}
		var targetStrings []string
		targetStrings = append(targetStrings, "Enter the <b>ID</b> of a target to delete it\n")
		for _, target := range targs {
			targetStrings = append(
				targetStrings,
				fmt.Sprintf(
					"<b>ID:</b> %v\n<b>Title:</b> %v\n<b>URL:</b> %v\n",
					target.ID,
					replaceHTML(target.Title),
					replaceHTML(target.URL)))
		}
		message := strings.Join(targetStrings, "\n")
		t.bot.SendDialogMessage(update.Message, message)
		return 2, true
	}
	if stepNumber == 2 {
		target, err := strconv.Atoi(update.Message.Text)
		if err != nil {
			t.bot.SendDialogMessage(update.Message, "Invalid ID, please try again")
			return 2, true
		}
		targetFromDB, err := t.bot.DB.GetTarget(target)
		if err != nil || targetFromDB.ChatID != update.Message.Chat.ID {
			t.bot.SendMessage(update.Message.Chat.ID, "No target with such ID found")
			return 0, false
		}
		err = t.bot.DB.DeleteTarget(target)
		if err != nil {
			t.bot.SendMessage(
				update.Message.Chat.ID,
				fmt.Sprintf(
					"Error while deleting the target, please contact the administrator: %v",
					t.bot.AdminNickname))
			return 0, false
		}
		t.bot.SendMessage(update.Message.Chat.ID, "Target was successfully deleted!")
		return 0, false
	}
	return 0, false
}

func (b *Bot) Dispatch(update *tgbotapi.Update) {
	if update.Message == nil {
		return
	}
	if _, ok := b.sessionMap[update.Message.Chat.ID]; !ok {
		b.sessionMap[update.Message.Chat.ID] = &session{}
		b.sessionMap[update.Message.Chat.ID].Stage = 1
		b.sessionMap[update.Message.Chat.ID].Dialog = nil
	}
	sess := b.sessionMap[update.Message.Chat.ID]
	if sess.Dialog != nil {
		var ok bool
		sess.Stage, ok = sess.Dialog.ContinueDialog(sess.Stage, *update, b.TgBot)
		if !ok {
			sess.Dialog = nil
		}
		return
	}
	if update.Message.Command() == "start" {
		b.SendMessage(
			update.Message.Chat.ID,
			"Hi!\nI'm a bot which can monitor sites' availability and notify you when a site goes down or up again.\n")
		return
	}
	if update.Message.Command() == "add" {
		b.StartDialog(update, &addNewTarget{
			bot: b,
		})
	}
	if update.Message.Command() == "targets" {
		targs, err := b.DB.GetCurrentTargets(update.Message.Chat.ID)
		if err != nil {
			b.SendMessage(
				update.Message.Chat.ID,
				fmt.Sprintf(
					"Error while retrieving the targets, please contact the administrator: %v",
					b.AdminNickname))
			return
		}
		if len(targs) == 0 {
			b.SendMessage(update.Message.Chat.ID, "No targets! Use /add to add one.")
			return
		}
		var targetStrings []string
		for _, target := range targs {
			status, ok, err := b.Monitor.StatusStore.GetStatus(target.ToTarget())
			if err != nil {
				b.SendMessage(
					update.Message.Chat.ID,
					fmt.Sprintf(
						"Error while retrieving the target's status, please contact the administrator: %v",
						b.AdminNickname))
				continue
			}

			var header string
			header = fmt.Sprintf(
				"<a href=\"%v\">%v</a>",
				replaceHTML(target.URL), replaceHTML(target.Title))

			var statusText string
			if ok {
				var emoji string
				if status.Type == monitor.StatusOK {
					emoji = okStatusEmoji
				} else {
					emoji = errorStatusEmoji
				}

				statusText = fmt.Sprintf(
					"%v %v (%v ms)",
					emoji, status.Type, int64(status.ResponseTime/time.Millisecond))
			} else {
				statusText = "N/A"
			}

			targetStrings = append(
				targetStrings, fmt.Sprintf("%v: %v", header, statusText))
		}
		message := strings.Join(targetStrings, "\n")
		b.SendMessage(update.Message.Chat.ID, message)
		return
	}
	if update.Message.Command() == "delete" {
		b.StartDialog(update, &deleteTarget{
			bot: b,
		})
	}
}

func (b *Bot) StartDialog(update *tgbotapi.Update, dialog dialog) {
	var ok bool
	b.sessionMap[update.Message.Chat.ID].Dialog = dialog
	b.sessionMap[update.Message.Chat.ID].Stage, ok = dialog.ContinueDialog(1, *update, b.TgBot)
	if !ok {
		b.sessionMap[update.Message.Chat.ID].Dialog = nil
	}
	return
}

func (b *Bot) Run() error {
	b.sessionMap = map[int64]*session{}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 0

	updates, err := b.TgBot.GetUpdatesChan(u)
	if err != nil {
		return err
	}

	for update := range updates {
		b.Dispatch(&update)
	}

	return nil
}
