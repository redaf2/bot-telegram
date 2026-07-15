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
	// Инициализируем бота
	bot, err := tgbotapi.NewBotAPI(b.cfg.Token)
	if err != nil {
		return fmt.Errorf("ошибка создания бота: %w", err)
	}

	b.api = bot
	bot.Debug = true
	log.Printf("✅ Бот @%s запущен!", bot.Self.UserName)

	// Проверяем наличие yt-dlp и ffmpeg
	b.down.CheckDependencies()

	// Запускаем очистку старых файлов
	go b.cleanTempFolder()

	// Настраиваем обновления
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	// Обработка сообщений
	for update := range updates {
		if update.Message == nil {
			continue
		}

		go b.handleMessage(update.Message)
	}

	return nil
}

func (b *Bot) handleMessage(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	if msg.IsCommand() && msg.Command() == "start" {
		reply := tgbotapi.NewMessage(chatID, "🎵 Отправь мне ссылку на видео, и я пришлю аудио!\n\nПоддерживаются: YouTube, SoundCloud, VK, TikTok и другие.")
		b.api.Send(reply)
		return
	}

	url := strings.TrimSpace(msg.Text)
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		reply := tgbotapi.NewMessage(chatID, "❌ Это не похоже на ссылку. Отправь валидный URL.")
		b.api.Send(reply)
		return
	}

	statusMsg := tgbotapi.NewMessage(chatID, "⏳ Скачиваю аудио...")
	sentStatus, _ := b.api.Send(statusMsg)

	b.down.SetProgressCallback(func(percent float64, downloaded, total, speed, eta string, spinner string) {
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
			speed, eta,
		)

		editMsg := tgbotapi.NewEditMessageText(chatID, sentStatus.MessageID, progressMsg)
		b.api.Send(editMsg)
	})

	audioFile, err := b.down.Download(url)
	if err != nil {
		errorText := fmt.Sprintf("❌ Ошибка:\n%s", err.Error())
		editMsg := tgbotapi.NewEditMessageText(chatID, sentStatus.MessageID, errorText)
		b.api.Send(editMsg)
		return
	}

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

// createProgressBar создаёт красивый прогресс-бар
func createProgressBar(percent float64, width int) string {
	filled := int(percent / 100 * float64(width))

	bar := strings.Repeat("█", filled)
	if filled < width {
		bar += strings.Repeat("░", width-filled)
	}

	return bar
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

// cleanTempFolder очищает старые файлы
func (b *Bot) cleanTempFolder() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		files, _ := filepath.Glob(filepath.Join(b.cfg.TempFolder, "*"))
		for _, file := range files {
			// Удаляем файлы старше 1 часа
			info, err := os.Stat(file)
			if err == nil && time.Since(info.ModTime()) > 1*time.Hour {
				os.Remove(file)
				log.Printf("🧹 Удалён старый файл: %s", filepath.Base(file))
			}
		}
	}
}
