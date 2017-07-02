package config

import (
	"github.com/BurntSushi/toml"
)

type Config struct {
	Monitor struct {
		Interval    int
		MaxParallel uint
		Timeout     int
		NotifyFirstOK bool
	}
	Database struct {
		Name string
	}
	Telegram struct {
		APIKey string
		Admin  string
	}
	Redis struct {
		Host string
		Port uint
		Pwd  string
		DB   int
	}
}

func ReadConfig(filename string) (*Config, error) {
	var conf Config
	_, err := toml.DecodeFile(filename, &conf)
	if err != nil {
		return nil, err
	}
	return &conf, nil
}
