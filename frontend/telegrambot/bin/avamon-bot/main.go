package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/jinzhu/gorm"
	"github.com/yamnikov-oleg/avamon-bot/frontend/telegrambot"
)

func main() {
	bot := telegrambot.Bot{}

	configPath := flag.String("config", "config.toml", "Path to the config file")
	flag.Parse()

	var err error
	bot.Config, err = telegrambot.ReadConfig(*configPath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	connection, err := gorm.Open("sqlite3", bot.Config.Database.Name)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	bot.DB = &telegrambot.TargetsDB{
		DB: connection,
	}
	bot.DB.Migrate()

	bot.TgBot, err = tgbotapi.NewBotAPI(bot.Config.Telegram.APIKey)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	bot.TgBot.Debug = bot.Config.Telegram.Debug

	err = bot.MonitorCreate()
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
