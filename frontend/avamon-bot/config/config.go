package config

import (
	"errors"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Database struct {
		Name string
	}
	Telegram struct {
		APIKey string
		Admin  string
	}
}

func ReadConfig(filename string) (*Config, error) {
	var conf Config
	mdata, err := toml.DecodeFile(filename, &conf)
	if err != nil {
		return nil, err
	}
	if result := mdata.IsDefined("database", "name"); !result {
		return nil, errors.New("File doesn't contain DBName line")
	}
	if result := mdata.IsDefined("telegram", "apikey"); !result {
		return nil, errors.New("APIKey not found")
	}
	if result := mdata.IsDefined("telegram", "admin"); !result {
		return nil, errors.New("Admin not found")
	}
	return &conf, nil
}
