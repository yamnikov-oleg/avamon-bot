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

func formatStatusUpdate(targ monitor.Target, stat monitor.Status) string {
	var output string
	// A line of green clovers emojis
	clover := strings.Repeat(string([]rune{0x2618, 0xfe0f}), 10) + "\n"
	// A line of red alarm emojis
	alarm := strings.Repeat(string([]rune{0x1f6a8}), 10) + "\n"
	title, url := replaceHTML(targ.Title, targ.URL)
	if stat.Type == monitor.StatusOK {
		output += clover
	} else {
		output += alarm
	}
	output += fmt.Sprintf("<b>%v:</b> <b>%v</b>\n\n", title, stat.Type)
	output += fmt.Sprintf("<b>URL:</b> %v\n", url)
	output += fmt.Sprintf("<b>Время ответа:</b> %v\n", stat.ResponseTime)
	if stat.Type != monitor.StatusOK {
		statusErr := strings.Replace(stat.Err.Error(), "<", "&lt;", -1)
		statusErr = strings.Replace(statusErr, ">", "&gt;", -1)
		output += fmt.Sprintf("<b>Сообщение:</b> %v\n", stat.Err)
	}
	if stat.Type == monitor.StatusHTTPError {
		output += fmt.Sprintf("<b>Статус HTTP:</b> %v %v\n", stat.HTTPStatusCode, http.StatusText(stat.HTTPStatusCode))
	}
	if stat.Type == monitor.StatusOK {
		output += clover
	} else {
		output += alarm
	}
	return output
}

func replaceHTML(title, url string) (string, string) {
	title = strings.Replace(title, "<", "&lt;", -1)
	title = strings.Replace(title, ">", "&gt;", -1)
	url = strings.Replace(url, "<", "&lt;", -1)
	url = strings.Replace(url, ">", "&gt;", -1)
	return title, url
}

func monitorStart(conf *Config, targets TargetsDB, bot *tgbotapi.BotAPI) (*monitor.Monitor, error) {
	mon := monitor.New(&targets)
	mon.Scheduler.Interval = time.Duration(conf.Monitor.Interval) * time.Second
	mon.Scheduler.ParallelPolls = conf.Monitor.MaxParallel
	mon.Scheduler.Poller.Timeout = time.Duration(conf.Monitor.Timeout) * time.Second
	mon.NotifyFirstOK = conf.Monitor.NotifyFirstOK

	ropts := monitor.RedisOptions{
		Host:     conf.Redis.Host,
		Port:     conf.Redis.Port,
		Password: conf.Redis.Pwd,
		DB:       conf.Redis.DB,
	}

	rs := monitor.NewRedisStore(ropts)
	if err := rs.Ping(); err != nil {
		return nil, err
	}
	mon.StatusStore = rs

	go func() {
		for upd := range mon.Updates {
			var rec Record
			targets.DB.First(&rec, upd.Target.ID)
			message := formatStatusUpdate(upd.Target, upd.Status)
			msg := tgbotapi.NewMessage(rec.ChatID, message)
			msg.ParseMode = tgbotapi.ModeHTML
			bot.Send(msg)
		}
	}()

	go func() {
		for err := range mon.Errors() {
			fmt.Println(err)
		}
	}()

	go mon.Run(nil)

	return mon, nil
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
			title, url := replaceHTML(target.Title, target.URL)
			targetStrings = append(targetStrings, fmt.Sprintf("<b>Идентификатор:</b> %v\n<b>Заголовок:</b> %v\n<b>URL:</b> %v\n", target.ID, title, url))
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
	configPath := flag.String("config", "config.toml", "Path to the config file")

	flag.Parse()
	conf, err := ReadConfig(*configPath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	connection, err := gorm.Open("sqlite3", conf.Database.Name)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	targets := TargetsDB{
		DB: connection,
	}
	targets.Migrate()
	bot, err := tgbotapi.NewBotAPI(conf.Telegram.APIKey)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	bot.Debug = conf.Telegram.Debug
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 0

	updates, err := bot.GetUpdatesChan(u)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	mon, err := monitorStart(conf, targets, bot)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

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
			sess.Stage, ok = sess.Dialog.ContinueDialog(sess.Stage, update, bot)
			if !ok {
				sess.Dialog = nil
			}
			continue
		}
		if update.Message.Command() == "start" {
			message := "Привет!\nЯ бот который умеет следить за доступностью сайтов.\n"

			msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
			bot.Send(msg)
			continue
		}
		if update.Message.Command() == "add" {
			var ok bool
			sess.Dialog = &addNewTarget{
				DB:   targets,
				conf: *conf,
			}
			sess.Stage, ok = sess.Dialog.ContinueDialog(1, update, bot)
			if !ok {
				sess.Dialog = nil
			}
			continue
		}
		if update.Message.Command() == "targets" {
			targs, err := targets.GetCurrentTargets(update.Message.Chat.ID)
			if err != nil {
				message := fmt.Sprintf("Ошибка получения целей, свяжитесь с администратором: %v", conf.Telegram.Admin)
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
				bot.Send(msg)
				continue
			}
			if len(targs) == 0 {
				message := "Целей не обнаружено!"
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
				bot.Send(msg)
				continue
			}
			var targetStrings []string
			for _, target := range targs {
				t := target.ToTarget()
				title, url := replaceHTML(target.Title, target.URL)
				status, ok, err := mon.StatusStore.GetStatus(t)
				if err != nil {
					message := fmt.Sprintf("Ошибка статуса целей, свяжитесь с администратором: %v", conf.Telegram.Admin)
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
					bot.Send(msg)
					continue
				}
				if !ok {
					targetStrings = append(targetStrings, fmt.Sprintf("<b>Заголовок:</b> %v\n<b>URL:</b> %v\n", title, url))
					continue
				}
				targetStrings = append(targetStrings, fmt.Sprintf("<b>Заголовок:</b> %v\n<b>URL:</b> %v\n<b>Статус:</b> %v\n<b>Время ответа:</b> %v\n", title, url, status.Type, status.ResponseTime))
			}
			message := strings.Join(targetStrings, "\n")
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
			msg.ParseMode = tgbotapi.ModeHTML
			bot.Send(msg)
			continue
		}
		if update.Message.Command() == "delete" {
			var ok bool
			sess.Dialog = &deleteTarget{
				DB:   targets,
				conf: *conf,
			}
			sess.Stage, ok = sess.Dialog.ContinueDialog(1, update, bot)
			if !ok {
				sess.Dialog = nil
			}
			continue
		}
	}
}
