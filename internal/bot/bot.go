package bot

import (
	"fmt"
	"log"
	"os"
	"strings"

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

	// Команда /start
	if msg.IsCommand() && msg.Command() == "start" {
		reply := tgbotapi.NewMessage(chatID, "🎵 Отправь мне ссылку на видео, и я пришлю аудио!")
		b.api.Send(reply)
		return
	}

	// Проверяем ссылку
	url := strings.TrimSpace(msg.Text)
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		reply := tgbotapi.NewMessage(chatID, "❌ Это не похоже на ссылку. Отправь валидный URL.")
		b.api.Send(reply)
		return
	}

	// Статус "загрузка"
	statusMsg := tgbotapi.NewMessage(chatID, "⏳ Скачиваю аудио...")
	sentStatus, _ := b.api.Send(statusMsg)

	// Устанавливаем колбэк для прогресса
	var lastProgressMsg string

	b.down.SetProgressCallback(func(percent float64, downloaded, total, speed, eta string) {
		// Создаём красивый прогресс-бар
		bar := createProgressBar(percent, 20)

		progressMsg := fmt.Sprintf(
			"📥 Загрузка аудио\n\n"+
				"┌─────────────────────────────┐\n"+
				"│ %s │ %5.1f%% │\n"+
				"├─────────────────────────────┤\n"+
				"│ ⬇️  %s / %s                  │\n"+
				"│ ⚡ %s │ ⏱️ %s       │\n"+
				"└─────────────────────────────┘",
			bar, percent,
			downloaded, total,
			speed, eta,
		)

		// Обновляем только если изменилось
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

	// Отправляем аудио
	audio, err := os.Open(audioFile)
	if err != nil {
		editMsg := tgbotapi.NewEditMessageText(chatID, sentStatus.MessageID, "❌ Не удалось открыть файл")
		b.api.Send(editMsg)
		return
	}
	defer audio.Close()
	defer os.Remove(audioFile)

	audioMsg := tgbotapi.NewAudio(chatID, tgbotapi.FileReader{
		Name:   audioFile[strings.LastIndex(audioFile, "/")+1:],
		Reader: audio,
	})
	audioMsg.Caption = "🎧 Вот твоё аудио!"

	_, err = b.api.Send(audioMsg)
	if err != nil {
		editMsg := tgbotapi.NewEditMessageText(chatID, sentStatus.MessageID, "❌ Не удалось отправить аудио")
		b.api.Send(editMsg)
		return
	}

	// Удаляем статусное сообщение
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
