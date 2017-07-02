package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/jinzhu/gorm"
	"github.com/yamnikov-oleg/avamon-bot/frontend/avamon-bot/config"
	"github.com/yamnikov-oleg/avamon-bot/frontend/avamon-bot/db"
	"github.com/yamnikov-oleg/avamon-bot/monitor"

	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

type session struct {
	Stage  int
	Dialog dialog
}

type dialog interface {
	ContinueDialog(stepNumber int, update tgbotapi.Update, bot *tgbotapi.BotAPI) (int, bool)
}

// TODO:
// Добавление
// Просмотр списка целей - без диалога
// Удаление цели

type addNewTarget struct {
	Title string
	URL   string
	DB    db.TargetsDB
	conf  config.Config
}

func (t *addNewTarget) ContinueDialog(stepNumber int, update tgbotapi.Update, bot *tgbotapi.BotAPI) (int, bool) {
	if stepNumber == 1 {
		message := "Введите заголовок цели"
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
		bot.Send(msg)
		return 2, true
	}
	if stepNumber == 2 {
		t.Title = update.Message.Text
		message := "Введите URL адрес цели"
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
		bot.Send(msg)
		return 3, true
	}
	if stepNumber == 3 {
		var message string
		if _, err := url.Parse(update.Message.Text); err != nil {
			message = "Ошибка ввода URL адреса, попробуйте еще раз"
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
			bot.Send(msg)
			return 3, true
		}
		t.URL = update.Message.Text
		err := t.DB.CreateTarget(db.Record{
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

func main() {
	configPath := flag.String("config", "config.toml", "Path to the config file")

	flag.Parse()
	conf, err := config.ReadConfig(*configPath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	connection, err := gorm.Open("sqlite3", conf.Database.Name)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	targets := db.TargetsDB{
		DB: connection,
	}
	targets.Migrate()
	bot, err := tgbotapi.NewBotAPI(conf.Telegram.APIKey)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	bot.Debug = true
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 0

	updates, err := bot.GetUpdatesChan(u)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = monitorStart(conf, targets, bot)
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
			var targetStrings []string
			for _, target := range targs {
				targetStrings = append(targetStrings, fmt.Sprintf("%v %v", target.Title, target.URL))
			}
			message := strings.Join(targetStrings, "\n")
			mgs := tgbotapi.NewMessage(update.Message.Chat.ID, message)
			bot.Send(mgs)
			continue
		}

	}
}

func monitorStart(conf *config.Config, targets db.TargetsDB, bot *tgbotapi.BotAPI) error {
	mon := monitor.New(&targets)
	mon.Scheduler.Interval = time.Duration(conf.Monitor.Interval) * time.Second
	mon.Scheduler.ParallelPolls = conf.Monitor.MaxParallel
	mon.Scheduler.Poller.Timeout = time.Duration(conf.Monitor.Timeout) * time.Second

	ropts := monitor.RedisOptions{
		Host:     conf.Redis.Host,
		Port:     conf.Redis.Port,
		Password: conf.Redis.Pwd,
		DB:       conf.Redis.DB,
	}

	rs := monitor.NewRedisStore(ropts)
	if err := rs.Ping(); err != nil {
		return err
	}
	mon.StatusStore = rs

	go func() {
		for upd := range mon.Updates {
			var rec db.Record
			var message string
			targets.DB.First(&rec, upd.Target.ID)
			if upd.Status.Type == monitor.StatusOK {
				message = fmt.Sprintf("%v is UP:\n", upd.Target)
			} else {
				message = fmt.Sprintf("%v is DOWN:\n", upd.Target)
			}
			message = message + upd.Status.ExpandedString()
			mgs := tgbotapi.NewMessage(rec.ChatID, message)
			bot.Send(mgs)
		}
	}()

	go func() {
		for err := range mon.Errors() {
			fmt.Println(err)
		}
	}()

	go mon.Run(nil)

	return nil
}
