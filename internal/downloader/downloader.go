package downloader

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ProgressCallback функция для обновления прогресса
type ProgressCallback func(percent float64, downloaded, total string, speed, eta string)

type Downloader struct {
	tempFolder string
	onProgress ProgressCallback
	lastUpdate time.Time
}

func NewDownloader(tempFolder string) *Downloader {
	os.MkdirAll(tempFolder, 0755)
	return &Downloader{
		tempFolder: tempFolder,
	}
}

// SetProgressCallback устанавливает функцию для обновления прогресса
func (d *Downloader) SetProgressCallback(callback ProgressCallback) {
	d.onProgress = callback
}

func (d *Downloader) CheckDependencies() {
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		log.Println("❌ yt-dlp не найден:", err)
	} else {
		log.Println("✅ yt-dlp найден")
	}

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		log.Println("❌ ffmpeg не найден:", err)
	} else {
		log.Println("✅ ffmpeg найден")
	}
}

func (d *Downloader) Download(url string) (string, error) {
	// Генерируем временный путь для скачивания
	timestamp := time.Now().UnixNano()
	tempPath := filepath.Join(d.tempFolder, fmt.Sprintf("temp_%d", timestamp))

	// Для MacOS с Apple Silicon используй полный путь
	ytDlpPath := "yt-dlp"
	// Если на Mac не работает, раскомментируй следующую строку:
	// ytDlpPath = "/opt/homebrew/bin/yt-dlp"

	// Создаём команду для скачивания
	cmd := exec.Command(
		ytDlpPath,
		"-f", "bestaudio/best",
		"--extract-audio",
		"--audio-format", "mp3",
		"--audio-quality", "192K",
		"-o", tempPath+".%(ext)s",
		"--progress",
		"--newline",
		url,
	)

	// Захватываем вывод прогресса
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("не удалось создать pipe для вывода: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("не удалось создать pipe для ошибок: %w", err)
	}

	// Запускаем команду
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("не удалось запустить yt-dlp: %w", err)
	}

	// Читаем прогресс в реальном времени
	go d.readProgress(stdout)

	// Читаем ошибки
	errBytes, _ := io.ReadAll(stderr)

	// Ждём завершения
	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("yt-dlp ошибка: %w\n%s", err, string(errBytes))
	}

	// Ищем скачанный файл
	files, err := filepath.Glob(tempPath + ".*")
	if err != nil || len(files) == 0 {
		return "", fmt.Errorf("не найден скачанный файл")
	}
	oldPath := files[0]

	// Получаем название видео через yt-dlp
	title, err := getVideoTitle(url)
	if err != nil {
		title = fmt.Sprintf("audio_%d", timestamp)
	}

	// Очищаем название от недопустимых символов
	safeTitle := sanitizeFilename(title)
	newPath := filepath.Join(d.tempFolder, safeTitle+".mp3")

	// Переименовываем файл
	if err := os.Rename(oldPath, newPath); err != nil {
		// Если не удалось переименовать, оставляем как есть
		return oldPath, nil
	}

	return newPath, nil
}

// readProgress читает и выводит прогресс загрузки
func (d *Downloader) readProgress(pipe io.ReadCloser) {
	scanner := bufio.NewScanner(pipe)

	// Регулярные выражения для парсинга прогресса
	percentRe := regexp.MustCompile(`\[download\]\s+(\d+\.\d+)%`)
	downloadedRe := regexp.MustCompile(`\[download\]\s+(\d+\.\d+)(KiB|MiB|GiB)`)
	speedRe := regexp.MustCompile(`at\s+(\d+\.\d+)(KiB|MiB|GiB)/s`)
	etaRe := regexp.MustCompile(`ETA\s+(\d+:\d+(?::\d+)?)`)
	totalRe := regexp.MustCompile(`of\s+~?\s*([\d.]+)\s*(KiB|MiB|GiB)`)

	var percent float64
	var downloaded, total float64
	var speed float64
	var eta string
	var totalUnit, downloadedUnit string

	for scanner.Scan() {
		line := scanner.Text()

		// Парсим процент
		if matches := percentRe.FindStringSubmatch(line); len(matches) > 1 {
			percent, _ = strconv.ParseFloat(matches[1], 64)
		}

		// Парсим скачанный размер
		if matches := downloadedRe.FindStringSubmatch(line); len(matches) > 2 {
			downloaded, _ = strconv.ParseFloat(matches[1], 64)
			downloadedUnit = matches[2]
		}

		// Парсим общий размер
		if matches := totalRe.FindStringSubmatch(line); len(matches) > 2 {
			total, _ = strconv.ParseFloat(matches[1], 64)
			totalUnit = matches[2]
		}

		// Парсим скорость
		if matches := speedRe.FindStringSubmatch(line); len(matches) > 2 {
			speed, _ = strconv.ParseFloat(matches[1], 64)
		}

		// Парсим ETA
		if matches := etaRe.FindStringSubmatch(line); len(matches) > 1 {
			eta = matches[1]
		}

		// Обновляем прогресс каждые 0.5 секунды
		if time.Since(d.lastUpdate) > 500*time.Millisecond && d.onProgress != nil {
			d.lastUpdate = time.Now()

			// Форматируем размеры
			downloadedStr := formatSize(downloaded, downloadedUnit)
			totalStr := formatSize(total, totalUnit)
			speedStr := formatSpeed(speed)

			// Вызываем колбэк
			d.onProgress(percent, downloadedStr, totalStr, speedStr, eta)
		}
	}
}

// getVideoTitle получает название видео через yt-dlp
func getVideoTitle(url string) (string, error) {
	cmd := exec.Command(
		"yt-dlp",
		"--get-title",
		"--no-warnings",
		url,
	)

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	title := strings.TrimSpace(string(output))
	return title, nil
}

// sanitizeFilename удаляет недопустимые символы из названия
func sanitizeFilename(filename string) string {
	// Заменяем недопустимые символы на "_"
	re := regexp.MustCompile(`[<>:"/\\|?*]|[\x00-\x1f]`)
	safe := re.ReplaceAllString(filename, "_")

	// Ограничиваем длину (макс 100 символов)
	if len(safe) > 100 {
		safe = safe[:100]
	}

	return safe
}

// formatSize форматирует размер в человеко-читаемый вид
func formatSize(size float64, unit string) string {
	// Конвертируем в байты
	var bytes float64
	switch unit {
	case "KiB":
		bytes = size * 1024
	case "MiB":
		bytes = size * 1024 * 1024
	case "GiB":
		bytes = size * 1024 * 1024 * 1024
	default:
		bytes = size
	}

	if bytes < 1024 {
		return fmt.Sprintf("%.0f B", bytes)
	} else if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", bytes/1024)
	} else if bytes < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", bytes/1024/1024)
	}
	return fmt.Sprintf("%.2f GB", bytes/1024/1024/1024)
}

// formatSpeed форматирует скорость
func formatSpeed(speed float64) string {
	if speed == 0 {
		return "⏳ вычисляется..."
	}

	if speed < 1024 {
		return fmt.Sprintf("%.1f KB/s", speed)
	}
	return fmt.Sprintf("%.1f MB/s", speed/1024)
}
