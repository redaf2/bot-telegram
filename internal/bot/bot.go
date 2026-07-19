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
			go b.handleCallback(update.CallbackQuery)
			continue
		}

		if update.Message == nil {
			continue
		}

		msg := update.Message
		chatID := msg.Chat.ID

		if msg.IsCommand() && msg.Command() == "start" {
			reply := tgbotapi.NewMessage(chatID, "🎬 Отправь мне ссылку на YouTube!\n\n"+
				"Я скачаю видео, покажу его, а ты выберешь, что скачать:\n"+
				"🎥 Видео 720p\n"+
				"🎧 Аудио (обычное)\n"+
				"🐢 Аудио (замедленное)")
			b.api.Send(reply)
			continue
		}

		url := strings.TrimSpace(msg.Text)
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			reply := tgbotapi.NewMessage(chatID, "❌ Это не ссылка. Отправь валидный URL.")
			b.api.Send(reply)
			continue
		}

		// Запускаем видео-фичу
		go b.processVideo(chatID, url)
	}

	return nil
}

// processVideo — скачивает видео и показывает его с кнопками
func (b *Bot) processVideo(chatID int64, url string) {
	statusMsg, _ := b.api.Send(tgbotapi.NewMessage(chatID, "⏳ Скачиваю видео..."))

	videoData, err := b.down.DownloadVideoOnly(url)
	if err != nil {
		b.api.Send(tgbotapi.NewMessage(chatID, "❌ Ошибка: "+err.Error()))
		return
	}
	defer os.Remove(videoData.Path)

	videoFile, err := os.Open(videoData.Path)
	if err != nil {
		b.api.Send(tgbotapi.NewMessage(chatID, "❌ Не удалось открыть видео"))
		return
	}
	defer videoFile.Close()

	videoMsg := tgbotapi.NewVideo(chatID, tgbotapi.FileReader{
		Name:   filepath.Base(videoData.Path),
		Reader: videoFile,
	})
	videoMsg.Caption = fmt.Sprintf("🎬 %s\n\nВыбери, что скачать:", videoData.Title)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🎥 Видео 720p", "video_"+videoData.Path),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🎧 Аудио (обычное)", "audio_normal_"+videoData.Path),
			tgbotapi.NewInlineKeyboardButtonData("🐢 Аудио (slow)", "audio_slow_"+videoData.Path),
		),
	)
	videoMsg.ReplyMarkup = keyboard

	b.api.Send(videoMsg)
	b.api.Send(tgbotapi.NewDeleteMessage(chatID, statusMsg.MessageID))
}

// handleCallback — обрабатывает нажатия на кнопки
func (b *Bot) handleCallback(callback *tgbotapi.CallbackQuery) {
	chatID := callback.Message.Chat.ID
	data := callback.Data

	b.api.Send(tgbotapi.NewCallback(callback.ID, "Обрабатываю..."))

	parts := strings.SplitN(data, "_", 3)
	if len(parts) < 2 {
		b.api.Send(tgbotapi.NewMessage(chatID, "❌ Ошибка: неверный формат"))
		return
	}

	action := parts[0]
	mode := parts[1]
	videoPath := strings.Join(parts[2:], "_")

	if _, err := os.Stat(videoPath); os.IsNotExist(err) {
		b.api.Send(tgbotapi.NewMessage(chatID, "❌ Видео уже удалено. Отправь ссылку заново."))
		return
	}

	switch action {
	case "video":
		b.sendFile(chatID, videoPath, "🎥 Вот твоё видео!")
	case "audio":
		slow := mode == "slow"
		b.processAudioFromVideo(chatID, videoPath, slow)
	}

	b.api.Send(tgbotapi.NewDeleteMessage(chatID, callback.Message.MessageID))
}

// processAudioFromVideo — извлекает аудио из видео
func (b *Bot) processAudioFromVideo(chatID int64, videoPath string, slow bool) {
	statusMsg, _ := b.api.Send(tgbotapi.NewMessage(chatID, "⏳ Извлекаю аудио..."))

	audioPath, err := b.down.ExtractAudioFromVideo(videoPath, slow)
	if err != nil {
		b.api.Send(tgbotapi.NewMessage(chatID, "❌ Ошибка извлечения аудио: "+err.Error()))
		return
	}
	defer os.Remove(audioPath)

	caption := "🎧 Вот твоё аудио!"
	if slow {
		caption = "🐢 Замедленное аудио (slowed)"
	}

	b.sendFile(chatID, audioPath, caption)
	b.api.Send(tgbotapi.NewDeleteMessage(chatID, statusMsg.MessageID))
}

// sendFile — отправляет любой файл
func (b *Bot) sendFile(chatID int64, path, caption string) {
	file, err := os.Open(path)
	if err != nil {
		b.api.Send(tgbotapi.NewMessage(chatID, "❌ Не удалось открыть файл"))
		return
	}
	defer file.Close()

	msg := tgbotapi.NewDocument(chatID, tgbotapi.FileReader{
		Name:   filepath.Base(path),
		Reader: file,
	})
	msg.Caption = caption
	b.api.Send(msg)
}

// cleanTempFolder — очищает временные файлы
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
