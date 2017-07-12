package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/jinzhu/gorm"
	"github.com/yamnikov-oleg/avamon-bot/frontend/telegrambot"
	"github.com/yamnikov-oleg/avamon-bot/monitor"
)

func monitorCreate(b *telegrambot.Bot, config *Config) error {
	mon := monitor.New(b.DB)
	mon.Scheduler.Interval = time.Duration(config.Monitor.Interval) * time.Second
	mon.Scheduler.ParallelPolls = config.Monitor.MaxParallel
	mon.Scheduler.Poller.Timeout = time.Duration(config.Monitor.Timeout) * time.Second
	mon.NotifyFirstOK = config.Monitor.NotifyFirstOK
	mon.Scheduler.Poller.TimeoutRetries = config.Monitor.TimeoutRetries
	mon.ExpirationTime = time.Duration(config.Monitor.ExpirationTime) * time.Second

	ropts := monitor.RedisOptions{
		Host:     config.Redis.Host,
		Port:     config.Redis.Port,
		Password: config.Redis.Pwd,
		DB:       config.Redis.DB,
	}

	rs := monitor.NewRedisStore(ropts)
	if err := rs.Ping(); err != nil {
		return err
	}
	mon.StatusStore = rs

	b.Monitor = mon

	return nil
}

func main() {
	bot := telegrambot.Bot{}

	configPath := flag.String("config", "config.toml", "Path to the config file")
	flag.Parse()

	config, err := ReadConfig(*configPath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if (config.Monitor.TimeoutRetries+1)*config.Monitor.Timeout >= config.Monitor.ExpirationTime {
		messageLines := []string{
			"Warning! Maximum number of timeouted poll attempts (%v+1 by %v seconds) takes greater or equal time",
			"to the status expiration time (%v seconds). This may lead to multiple timeout notifications.",
			"Increase expiration time or decrease timeout to fix this issue.",
			"\n",
		}
		fmt.Printf(
			strings.Join(messageLines, " "),
			config.Monitor.TimeoutRetries,
			config.Monitor.Timeout,
			config.Monitor.ExpirationTime,
		)
	}

	connection, err := gorm.Open("sqlite3", config.Database.Name)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	bot.DB = &telegrambot.TargetsDB{
		DB: connection,
	}
	bot.DB.Migrate()

	bot.TgBot, err = tgbotapi.NewBotAPI(config.Telegram.APIKey)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	bot.TgBot.Debug = config.Telegram.Debug
	bot.AdminNickname = config.Telegram.Admin

	err = monitorCreate(&bot, config)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	bot.MonitorStart()

	err = bot.Run()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
