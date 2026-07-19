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

// ProgressCallback с добавленным spinner
type ProgressCallback func(percent float64, downloaded, total string, speed, eta string, spinner string)

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
	timestamp := time.Now().UnixNano()
	tempPath := filepath.Join(d.tempFolder, fmt.Sprintf("temp_%d", timestamp))

	ytDlpPath := "yt-dlp"
	cmd := exec.Command(
		ytDlpPath,
		"--cookies", "/app/cookies.txt",
		"-f", "bestaudio/best",
		"--extract-audio",
		"--audio-format", "mp3",
		"--audio-quality", "192K",
		"-o", tempPath+".%(ext)s",
		"--progress",
		"--newline",
		url,
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("не удалось создать pipe для вывода: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("не удалось создать pipe для ошибок: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("не удалось запустить yt-dlp: %w", err)
	}

	go d.readProgress(stdout)
	errBytes, _ := io.ReadAll(stderr)

	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("yt-dlp ошибка: %w\n%s", err, string(errBytes))
	}

	files, err := filepath.Glob(tempPath + ".*")
	if err != nil || len(files) == 0 {
		return "", fmt.Errorf("не найден скачанный файл")
	}
	oldPath := files[0]

	title, err := getVideoTitle(url)
	if err != nil {
		title = fmt.Sprintf("audio_%d", timestamp)
	}
	safeTitle := sanitizeFilename(title)
	newPath := filepath.Join(d.tempFolder, safeTitle+".mp3")

	if err := os.Rename(oldPath, newPath); err != nil {
		return oldPath, nil
	}
	return newPath, nil
}

func (d *Downloader) readProgress(pipe io.ReadCloser) {
	scanner := bufio.NewScanner(pipe)
	spinner := []string{"🔄", "↩️", "⏩", "⏫", "⏪", "↪️", "⏬"}
	spinnerIdx := 0

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

	if d.onProgress != nil {
		d.onProgress(0, "0 B", "0 B", "⏳", "вычисляется...", "🔄")
	}

	for scanner.Scan() {
		line := scanner.Text()

		if matches := percentRe.FindStringSubmatch(line); len(matches) > 1 {
			percent, _ = strconv.ParseFloat(matches[1], 64)
		}
		if matches := downloadedRe.FindStringSubmatch(line); len(matches) > 2 {
			downloaded, _ = strconv.ParseFloat(matches[1], 64)
			downloadedUnit = matches[2]
		}
		if matches := totalRe.FindStringSubmatch(line); len(matches) > 2 {
			total, _ = strconv.ParseFloat(matches[1], 64)
			totalUnit = matches[2]
		}
		if matches := speedRe.FindStringSubmatch(line); len(matches) > 2 {
			speed, _ = strconv.ParseFloat(matches[1], 64)
		}
		if matches := etaRe.FindStringSubmatch(line); len(matches) > 1 {
			eta = matches[1]
		}

		if time.Since(d.lastUpdate) > 300*time.Millisecond && d.onProgress != nil {
			d.lastUpdate = time.Now()
			spinnerIdx = (spinnerIdx + 1) % len(spinner)

			downloadedStr := formatSize(downloaded, downloadedUnit)
			totalStr := formatSize(total, totalUnit)
			speedStr := formatSpeed(speed)

			d.onProgress(percent, downloadedStr, totalStr, speedStr, eta, spinner[spinnerIdx])
		}
	}
}

func getVideoTitle(url string) (string, error) {
	cmd := exec.Command("yt-dlp", "--get-title", "--no-warnings", url)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func sanitizeFilename(filename string) string {
	re := regexp.MustCompile(`[<>:"/\\|?*]|[\x00-\x1f]`)
	safe := re.ReplaceAllString(filename, "_")
	if len(safe) > 100 {
		safe = safe[:100]
	}
	return safe
}

func formatSize(size float64, unit string) string {
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

func formatSpeed(speed float64) string {
	if speed == 0 {
		return "⏳ вычисляется..."
	}
	if speed < 1024 {
		return fmt.Sprintf("%.1f KB/s", speed)
	}
	return fmt.Sprintf("%.1f MB/s", speed/1024)
}

// SlowAudio замедляет аудио и добавляет реверберацию
func (d *Downloader) SlowAudio(inputPath string, percent float64) (string, error) {
	ext := filepath.Ext(inputPath)
	base := strings.TrimSuffix(inputPath, ext)
	outputPath := fmt.Sprintf("%s_slowed_reverb_%.0f%s", base, percent*100, ext)

	// Шаг 1: Измеряем и нормализуем громкость, затем замедляем и добавляем эффекты
	cmd := exec.Command(
		"ffmpeg",
		"-i", inputPath,
		"-af",
		fmt.Sprintf(
			"volume=6.4dB,"+
				"atempo=%.1f,"+
				"aecho=0.06:0.9:50:0.7,"+
				"firequalizer=gain_entry='entry(0,24);entry(250,12);entry(500,4);entry(1000,2);entry(4000,-4);entry(16000,-20)'",
			percent,
		),
		"-y",
		outputPath,
	)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("не удалось создать pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("не удалось запустить ffmpeg: %w", err)
	}

	errBytes, _ := io.ReadAll(stderr)
	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("ffmpeg ошибка: %w\n%s", err, string(errBytes))
	}

	log.Printf("✅ Slowed + Reverb готово: %s (%.0f%%)", outputPath, percent*100)
	return outputPath, nil
}
