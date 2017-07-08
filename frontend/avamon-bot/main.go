package main

import (
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/jinzhu/gorm"
	"github.com/yamnikov-oleg/avamon-bot/monitor"

	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

func replaceHTML(input string) string {
	input = strings.Replace(input, "<", "&lt;", -1)
	input = strings.Replace(input, ">", "&gt;", -1)
	return input
}

type Bot struct {
	Config  *Config
	DB      *TargetsDB
	TgBot   *tgbotapi.BotAPI
	Monitor *monitor.Monitor
}

func (b *Bot) formatStatusUpdate(target monitor.Target, status monitor.Status) string {
	var output string
	var sign string

	if status.Type == monitor.StatusOK {
		// A line of green clovers emojis
		sign = strings.Repeat(string([]rune{0x2618, 0xfe0f}), 10) + "\n"
	} else {
		// A line of red alarm emojis
		sign = strings.Repeat(string([]rune{0x1f6a8}), 10) + "\n"
	}

	output += sign
	output += fmt.Sprintf("<b>%v:</b> <b>%v</b>\n\n", replaceHTML(target.Title), status.Type)
	output += fmt.Sprintf("<b>URL:</b> %v\n", replaceHTML(target.URL))
	output += fmt.Sprintf("<b>Время ответа:</b> %v\n", status.ResponseTime)

	if status.Type != monitor.StatusOK {
		output += fmt.Sprintf("<b>Сообщение:</b> %v\n", replaceHTML(status.Err.Error()))
	}
	if status.Type == monitor.StatusHTTPError {
		output += fmt.Sprintf("<b>Статус HTTP:</b> %v %v\n", status.HTTPStatusCode, http.StatusText(status.HTTPStatusCode))
	}
	output += sign

	return output
}

func (b *Bot) monitorCreate() error {
	mon := monitor.New(b.DB)
	mon.Scheduler.Interval = time.Duration(b.Config.Monitor.Interval) * time.Second
	mon.Scheduler.ParallelPolls = b.Config.Monitor.MaxParallel
	mon.Scheduler.Poller.Timeout = time.Duration(b.Config.Monitor.Timeout) * time.Second
	mon.NotifyFirstOK = b.Config.Monitor.NotifyFirstOK

	ropts := monitor.RedisOptions{
		Host:     b.Config.Redis.Host,
		Port:     b.Config.Redis.Port,
		Password: b.Config.Redis.Pwd,
		DB:       b.Config.Redis.DB,
	}

	rs := monitor.NewRedisStore(ropts)
	if err := rs.Ping(); err != nil {
		return err
	}
	mon.StatusStore = rs

	b.Monitor = mon

	return nil
}

func (b *Bot) monitorStart() {
	go func() {
		for upd := range b.Monitor.Updates {
			var rec Record
			b.DB.DB.First(&rec, upd.Target.ID)
			message := b.formatStatusUpdate(upd.Target, upd.Status)
			msg := tgbotapi.NewMessage(rec.ChatID, message)
			msg.ParseMode = tgbotapi.ModeHTML
			b.TgBot.Send(msg)
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
	DB    TargetsDB
	conf  Config
}

func (t *addNewTarget) ContinueDialog(stepNumber int, update tgbotapi.Update, bot *tgbotapi.BotAPI) (int, bool) {
	if stepNumber == 1 {
		message := "Введите заголовок цели"
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
		msg.ReplyToMessageID = update.Message.MessageID
		msg.ReplyMarkup = tgbotapi.ForceReply{
			ForceReply: true,
			Selective:  true,
		}
		bot.Send(msg)
		return 2, true
	}
	if stepNumber == 2 {
		t.Title = update.Message.Text
		message := "Введите URL адрес цели"
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
		msg.ReplyToMessageID = update.Message.MessageID
		msg.ReplyMarkup = tgbotapi.ForceReply{
			ForceReply: true,
			Selective:  true,
		}
		bot.Send(msg)
		return 3, true
	}
	if stepNumber == 3 {
		var message string
		if _, err := url.Parse(update.Message.Text); err != nil {
			message = "Ошибка ввода URL адреса, попробуйте еще раз"
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
			msg.ReplyToMessageID = update.Message.MessageID
			msg.ReplyMarkup = tgbotapi.ForceReply{
				ForceReply: true,
				Selective:  true,
			}
			bot.Send(msg)
			return 3, true
		}
		t.URL = update.Message.Text
		err := t.DB.CreateTarget(Record{
			ChatID: update.Message.Chat.ID,
			Title:  t.Title,
			URL:    t.URL,
		})
		if err != nil {
			message = fmt.Sprintf("Ошибка добавления цели, свяжитесь с администратором: %v", t.conf.Telegram.Admin)
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
			bot.Send(msg)
			return 0, false
		}
		message = "Цель успешно добавлена"
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
		bot.Send(msg)
		return 0, false
	}
	return 0, false
}

type deleteTarget struct {
	DB   TargetsDB
	conf Config
}

func (t *deleteTarget) ContinueDialog(stepNumber int, update tgbotapi.Update, bot *tgbotapi.BotAPI) (int, bool) {
	if stepNumber == 1 {
		targs, err := t.DB.GetCurrentTargets(update.Message.Chat.ID)
		if err != nil {
			message := fmt.Sprintf("Ошибка получения целей, свяжитесь с администратором: %v", t.conf.Telegram.Admin)
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
			bot.Send(msg)
			return 0, false
		}
		if len(targs) == 0 {
			message := "Целей не обнаружено!"
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
			bot.Send(msg)
			return 0, false
		}
		var targetStrings []string
		targetStrings = append(targetStrings, "Введите <b>идентификатор</b> цели для удаления\n")
		for _, target := range targs {
			targetStrings = append(
				targetStrings,
				fmt.Sprintf(
					"<b>Идентификатор:</b> %v\n<b>Заголовок:</b> %v\n<b>URL:</b> %v\n",
					target.ID,
					replaceHTML(target.Title),
					replaceHTML(target.URL)))
		}
		message := strings.Join(targetStrings, "\n")
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
		msg.ParseMode = tgbotapi.ModeHTML
		msg.ReplyToMessageID = update.Message.MessageID
		msg.ReplyMarkup = tgbotapi.ForceReply{
			ForceReply: true,
			Selective:  true,
		}
		bot.Send(msg)
		return 2, true
	}
	if stepNumber == 2 {
		target, err := strconv.Atoi(update.Message.Text)
		if err != nil {
			message := "Ошибка ввода идентификатора"
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
			msg.ReplyToMessageID = update.Message.MessageID
			msg.ReplyMarkup = tgbotapi.ForceReply{
				ForceReply: true,
				Selective:  true,
			}
			bot.Send(msg)
			return 2, true
		}
		targetFromDB := Record{}
		err = t.DB.DB.Where("ID = ?", target).First(&targetFromDB).Error
		if err != nil || targetFromDB.ChatID != update.Message.Chat.ID {
			message := "Цель не найдена"
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
			bot.Send(msg)
			return 0, false
		}
		err = t.DB.DB.Where("ID = ?", target).Delete(Record{}).Error
		if err != nil {
			message := fmt.Sprintf("Ошибка удаления цели, свяжитесь с администратором: %v", t.conf.Telegram.Admin)
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
			bot.Send(msg)
			return 0, false
		}
		message := "Цель успешно удалена!"
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
		bot.Send(msg)
		return 0, false
	}
	return 0, false
}

func main() {
	bot := Bot{}

	configPath := flag.String("config", "config.toml", "Path to the config file")
	flag.Parse()

	var err error
	bot.Config, err = ReadConfig(*configPath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	connection, err := gorm.Open("sqlite3", bot.Config.Database.Name)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	bot.DB = &TargetsDB{
		DB: connection,
	}
	bot.DB.Migrate()

	bot.TgBot, err = tgbotapi.NewBotAPI(bot.Config.Telegram.APIKey)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	bot.TgBot.Debug = bot.Config.Telegram.Debug
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 0

	updates, err := bot.TgBot.GetUpdatesChan(u)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = bot.monitorCreate()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	bot.monitorStart()

	var sessionMap = map[int64]*session{}
	for update := range updates {
		if update.Message == nil {
			continue
		}
		if _, ok := sessionMap[update.Message.Chat.ID]; !ok {
			sessionMap[update.Message.Chat.ID] = &session{}
			sessionMap[update.Message.Chat.ID].Stage = 1
			sessionMap[update.Message.Chat.ID].Dialog = nil
		}
		sess := sessionMap[update.Message.Chat.ID]
		if sess.Dialog != nil {
			var ok bool
			sess.Stage, ok = sess.Dialog.ContinueDialog(sess.Stage, update, bot.TgBot)
			if !ok {
				sess.Dialog = nil
			}
			continue
		}
		if update.Message.Command() == "start" {
			message := "Привет!\nЯ бот который умеет следить за доступностью сайтов.\n"

			msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
			bot.TgBot.Send(msg)
			continue
		}
		if update.Message.Command() == "add" {
			var ok bool
			sess.Dialog = &addNewTarget{
				DB:   *bot.DB,
				conf: *bot.Config,
			}
			sess.Stage, ok = sess.Dialog.ContinueDialog(1, update, bot.TgBot)
			if !ok {
				sess.Dialog = nil
			}
			continue
		}
		if update.Message.Command() == "targets" {
			targs, err := bot.DB.GetCurrentTargets(update.Message.Chat.ID)
			if err != nil {
				message := fmt.Sprintf("Ошибка получения целей, свяжитесь с администратором: %v", bot.Config.Telegram.Admin)
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
				bot.TgBot.Send(msg)
				continue
			}
			if len(targs) == 0 {
				message := "Целей не обнаружено!"
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
				bot.TgBot.Send(msg)
				continue
			}
			var targetStrings []string
			for _, target := range targs {
				status, ok, err := bot.Monitor.StatusStore.GetStatus(target.ToTarget())
				if err != nil {
					message := fmt.Sprintf(
						"Ошибка статуса целей, свяжитесь с администратором: %v",
						bot.Config.Telegram.Admin)
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
					bot.TgBot.Send(msg)
					continue
				}
				if !ok {
					targetStrings = append(
						targetStrings,
						fmt.Sprintf(
							"<b>Заголовок:</b> %v\n<b>URL:</b> %v\n",
							replaceHTML(target.Title),
							replaceHTML(target.URL)))
					continue
				}
				targetStrings = append(
					targetStrings,
					fmt.Sprintf(
						"<b>Заголовок:</b> %v\n<b>URL:</b> %v\n<b>Статус:</b> %v\n<b>Время ответа:</b> %v\n",
						replaceHTML(target.Title),
						replaceHTML(target.URL),
						status.Type, status.ResponseTime))
			}
			message := strings.Join(targetStrings, "\n")
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
			msg.ParseMode = tgbotapi.ModeHTML
			bot.TgBot.Send(msg)
			continue
		}
		if update.Message.Command() == "delete" {
			var ok bool
			sess.Dialog = &deleteTarget{
				DB:   *bot.DB,
				conf: *bot.Config,
			}
			sess.Stage, ok = sess.Dialog.ContinueDialog(1, update, bot.TgBot)
			if !ok {
				sess.Dialog = nil
			}
			continue
		}
	}
}
