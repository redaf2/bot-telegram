package downloader

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ProgressCallback функция для обновления прогресса
type ProgressCallback func(percent float64, downloaded, total string, speed, eta string, spinner string)

type Downloader struct {
	tempFolder string
	onProgress ProgressCallback
	lastUpdate time.Time
}

type VideoData struct {
	Path     string
	Title    string
	FileSize int64
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

// DownloadVideoOnly — скачивает только видео (без извлечения аудио)
func (d *Downloader) DownloadVideoOnly(url string) (*VideoData, error) {
	timestamp := time.Now().UnixNano()
	tempPath := filepath.Join(d.tempFolder, fmt.Sprintf("video_%d", timestamp))

	title, _ := getVideoTitle(url)
	if title == "" {
		title = fmt.Sprintf("video_%d", timestamp)
	}
	safeTitle := sanitizeFilename(title)

	cmd := exec.Command(
		"yt-dlp",
		"--user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"-f", "bestvideo[height<=720][ext=mp4]+bestaudio[ext=m4a]/best[height<=720][ext=mp4]",
		"-o", tempPath+".%(ext)s",
		"--progress",
		"--newline",
		url,
	)

	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("не удалось запустить yt-dlp: %w", err)
	}

	errBytes, _ := io.ReadAll(stderr)
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("yt-dlp ошибка: %w\n%s", err, string(errBytes))
	}

	files, _ := filepath.Glob(tempPath + ".*")
	if len(files) == 0 {
		return nil, fmt.Errorf("не найден скачанный файл")
	}

	ext := filepath.Ext(files[0])
	newPath := filepath.Join(d.tempFolder, safeTitle+ext)
	os.Rename(files[0], newPath)

	info, _ := os.Stat(newPath)

	return &VideoData{
		Path:     newPath,
		Title:    safeTitle,
		FileSize: info.Size(),
	}, nil
}

// ExtractAudioFromVideo — извлекает аудио из видео (с опцией замедления)
func (d *Downloader) ExtractAudioFromVideo(videoPath string, slow bool) (string, error) {
	ext := filepath.Ext(videoPath)
	base := strings.TrimSuffix(videoPath, ext)

	var outputPath string
	var cmd *exec.Cmd

	if !slow {
		outputPath = base + ".mp3"
		cmd = exec.Command(
			"ffmpeg",
			"-i", videoPath,
			"-q:a", "0",
			"-map", "a",
			"-y",
			outputPath,
		)
	} else {
		outputPath = fmt.Sprintf("%s_slowed.mp3", base)
		cmd = exec.Command(
			"ffmpeg",
			"-i", videoPath,
			"-q:a", "0",
			"-map", "a",
			"-af", "atempo=0.8, aecho=0.3:0.7:100:0.2, bass=g=3:f=110:w=0.5",
			"-y",
			outputPath,
		)
	}

	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("не удалось запустить ffmpeg: %w", err)
	}

	errBytes, _ := io.ReadAll(stderr)
	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("ffmpeg ошибка: %w\n%s", err, string(errBytes))
	}

	return outputPath, nil
}

// getVideoTitle получает название видео
func getVideoTitle(url string) (string, error) {
	cmd := exec.Command("yt-dlp", "--get-title", "--no-warnings", url)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// sanitizeFilename удаляет недопустимые символы
func sanitizeFilename(filename string) string {
	re := regexp.MustCompile(`[<>:"/\\|?*]|[\x00-\x1f]`)
	safe := re.ReplaceAllString(filename, "_")
	if len(safe) > 100 {
		safe = safe[:100]
	}
	return safe
}
