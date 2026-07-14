package config

import (
	"os"
)

type Config struct {
	Token      string
	TempFolder string
}

func New() *Config {
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		panic("BOT_TOKEN не установлен в .env файле")
	}

	return &Config{
		Token:      token,
		TempFolder: "temp",
	}
}
