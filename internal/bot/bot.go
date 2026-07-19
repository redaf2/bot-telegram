package bot

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"audiobot/internal/config"
	"audiobot/internal/downloader"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	api  *tgbotapi.BotAPI
	cfg  *config.Config
	down *downloader.Downloader
}

func NewBot(cfg *config.Config) *Bot {
	return &Bot{
		cfg:  cfg,
		down: downloader.NewDownloader(cfg.TempFolder),
	}
}

func (b *Bot) Run() error {
	bot, err := tgbotapi.NewBotAPI(b.cfg.Token)
	if err != nil {
		return fmt.Errorf("ошибка создания бота: %w", err)
	}

	b.api = bot
	bot.Debug = true
	log.Printf("✅ Бот @%s запущен!", bot.Self.UserName)

	b.down.CheckDependencies()
	go b.cleanTempFolder()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		// Обработка нажатий на кнопки
		if update.CallbackQuery != nil {
			callback := update.CallbackQuery
			data := callback.Data

			b.api.Send(tgbotapi.NewCallback(callback.ID, "Обрабатываю..."))

			// Формат: normal_https://... или slow_https://...
			parts := strings.SplitN(data, "_", 2)
			if len(parts) != 2 {
				b.api.Send(tgbotapi.NewMessage(callback.Message.Chat.ID, "❌ Ошибка: неверный формат"))
				continue
			}

			mode := parts[0] // "normal" или "slow"
			url := parts[1]

			// Удаляем сообщение с кнопками
			deleteMsg := tgbotapi.NewDeleteMessage(callback.Message.Chat.ID, callback.Message.MessageID)
			b.api.Send(deleteMsg)

			// Обрабатываем аудио (для замедления используем 0.8 = 20%)
			go b.processAudio(callback.Message.Chat.ID, url, mode == "slow", 0.8)
			continue
		}

		if update.Message == nil {
			continue
		}

		msg := update.Message
		chatID := msg.Chat.ID

		if msg.IsCommand() && msg.Command() == "start" {
			reply := tgbotapi.NewMessage(chatID, "🎵 Отправь мне ссылку на видео!\n\n"+
				"Я пришлю аудио. Ты сможешь выбрать:\n"+
				"🎧 Обычная версия\n"+
				"🐢 Замедленная + реверберация (slowed + reverb)")
			b.api.Send(reply)
			continue
		}

		url := strings.TrimSpace(msg.Text)
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			reply := tgbotapi.NewMessage(chatID, "❌ Это не похоже на ссылку. Отправь валидный URL.")
			b.api.Send(reply)
			continue
		}

		// --- 2 КНОПКИ ---
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("🎧 Обычная", "normal_"+url),
				tgbotapi.NewInlineKeyboardButtonData("🐢 Замедленная", "slow_"+url),
			),
		)

		msgWithButtons := tgbotapi.NewMessage(chatID, "🎵 Что сделать с этим видео?")
		msgWithButtons.ReplyMarkup = keyboard
		b.api.Send(msgWithButtons)
	}

	return nil
}

func (b *Bot) processAudio(chatID int64, url string, slow bool, speed float64) {
	statusMsg := tgbotapi.NewMessage(chatID, "⏳ Скачиваю аудио...")
	sentStatus, _ := b.api.Send(statusMsg)

	var lastProgressMsg string
	b.down.SetProgressCallback(func(percent float64, downloaded, total, speedStr, eta string, spinner string) {
		if percent == 0 && downloaded == "0 B" {
			progressMsg := fmt.Sprintf("%s Обработка ссылки...", spinner)
			editMsg := tgbotapi.NewEditMessageText(chatID, sentStatus.MessageID, progressMsg)
			b.api.Send(editMsg)
			return
		}

		bar := createProgressBarSimple(percent, 12)
		progressMsg := fmt.Sprintf(
			"📥 Скачивание: [%s] %.0f%%\n📦 %s / %s\n⚡ %s | ⏱️ %s",
			bar, percent,
			downloaded, total,
			speedStr, eta,
		)

		if progressMsg != lastProgressMsg {
			lastProgressMsg = progressMsg
			editMsg := tgbotapi.NewEditMessageText(chatID, sentStatus.MessageID, progressMsg)
			b.api.Send(editMsg)
		}
	})

	// Скачиваем аудио
	audioFile, err := b.down.Download(url)
	if err != nil {
		errorText := fmt.Sprintf("❌ Ошибка:\n%s", err.Error())
		editMsg := tgbotapi.NewEditMessageText(chatID, sentStatus.MessageID, errorText)
		b.api.Send(editMsg)
		return
	}

	// Если запрошено замедление + reverb
	if slow {
		editMsg := tgbotapi.NewEditMessageText(chatID, sentStatus.MessageID, "🐢 Замедляю + добавляю реверберацию...")
		b.api.Send(editMsg)

		slowFile, err := b.down.SlowAudio(audioFile, speed)
		if err != nil {
			errorText := fmt.Sprintf("❌ Ошибка замедления:\n%s", err.Error())
			editMsg := tgbotapi.NewEditMessageText(chatID, sentStatus.MessageID, errorText)
			b.api.Send(editMsg)
			return
		}

		audio, err := os.Open(slowFile)
		if err != nil {
			editMsg := tgbotapi.NewEditMessageText(chatID, sentStatus.MessageID, "❌ Не удалось открыть файл")
			b.api.Send(editMsg)
			return
		}
		defer audio.Close()
		defer os.Remove(slowFile)
		defer os.Remove(audioFile)

		audioMsg := tgbotapi.NewAudio(chatID, tgbotapi.FileReader{
			Name:   filepath.Base(slowFile),
			Reader: audio,
		})
		audioMsg.Caption = "🐢 Замедленная + реверберация (slowed + reverb)"

		_, err = b.api.Send(audioMsg)
		if err != nil {
			editMsg := tgbotapi.NewEditMessageText(chatID, sentStatus.MessageID, "❌ Не удалось отправить аудио")
			b.api.Send(editMsg)
			return
		}

		deleteMsg := tgbotapi.NewDeleteMessage(chatID, sentStatus.MessageID)
		b.api.Send(deleteMsg)
		return
	}

	// Отправляем обычное аудио
	audio, err := os.Open(audioFile)
	if err != nil {
		editMsg := tgbotapi.NewEditMessageText(chatID, sentStatus.MessageID, "❌ Не удалось открыть файл")
		b.api.Send(editMsg)
		return
	}
	defer audio.Close()
	defer os.Remove(audioFile)

	audioMsg := tgbotapi.NewAudio(chatID, tgbotapi.FileReader{
		Name:   filepath.Base(audioFile),
		Reader: audio,
	})
	audioMsg.Caption = "🎧 Вот твоё аудио!"

	_, err = b.api.Send(audioMsg)
	if err != nil {
		editMsg := tgbotapi.NewEditMessageText(chatID, sentStatus.MessageID, "❌ Не удалось отправить аудио")
		b.api.Send(editMsg)
		return
	}

	deleteMsg := tgbotapi.NewDeleteMessage(chatID, sentStatus.MessageID)
	b.api.Send(deleteMsg)
}

func createProgressBarSimple(percent float64, width int) string {
	filled := int(percent / 100 * float64(width))
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("█", filled)
	bar += strings.Repeat("░", width-filled)
	return bar
}

func (b *Bot) cleanTempFolder() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		files, _ := filepath.Glob(filepath.Join(b.cfg.TempFolder, "*"))
		for _, file := range files {
			info, err := os.Stat(file)
			if err == nil && time.Since(info.ModTime()) > 1*time.Hour {
				os.Remove(file)
				log.Printf("🧹 Удалён старый файл: %s", filepath.Base(file))
			}
		}
	}
}
