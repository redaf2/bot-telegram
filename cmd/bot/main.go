package main

import (
	"log"

	"audiobot/internal/bot"
	"audiobot/internal/config"

	"github.com/joho/godotenv"
)

func main() {
	// Загружаем .env
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️ .env файл не найден, используем переменные окружения")
	}

	// Загружаем конфиг
	cfg := config.New()

	// Создаём и запускаем бота
	app := bot.NewBot(cfg)

	if err := app.Run(); err != nil {
		log.Fatal("Ошибка при запуске бота:", err)
	}
}
