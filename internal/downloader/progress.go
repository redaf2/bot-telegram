package downloader

import (
	"fmt"
	"strings"
	"time"
)

// ProgressBar структура для отслеживания прогресса
type ProgressBar struct {
	total     int64
	current   int64
	startTime time.Time
	lastTime  time.Time
	lastBytes int64
	speed     float64
}

// NewProgressBar создаёт новый прогресс-бар
func NewProgressBar(total int64) *ProgressBar {
	return &ProgressBar{
		total:     total,
		startTime: time.Now(),
		lastTime:  time.Now(),
	}
}

// Update обновляет прогресс
func (p *ProgressBar) Update(current int64) {
	p.current = current

	// Вычисляем скорость
	now := time.Now()
	elapsed := now.Sub(p.lastTime).Seconds()
	if elapsed > 0 {
		bytesDiff := current - p.lastBytes
		p.speed = float64(bytesDiff) / elapsed / 1024 // KB/s
	}

	p.lastTime = now
	p.lastBytes = current
}

// GetPercentage возвращает процент загрузки
func (p *ProgressBar) GetPercentage() float64 {
	if p.total == 0 {
		return 0
	}
	return float64(p.current) / float64(p.total) * 100
}

// GetProgressBar возвращает красивую строку прогресс-бара
func (p *ProgressBar) GetProgressBar(width int) string {
	percent := p.GetPercentage()
	filled := int(percent / 100 * float64(width))

	bar := strings.Repeat("█", filled)
	if filled < width {
		bar += strings.Repeat("░", width-filled)
	}

	return bar
}

// GetSpeed возвращает скорость в человеко-читаемом формате
func (p *ProgressBar) GetSpeed() string {
	if p.speed < 1024 {
		return fmt.Sprintf("%.0f KB/s", p.speed)
	}
	return fmt.Sprintf("%.2f MB/s", p.speed/1024)
}

// GetSize возвращает размер в человеко-читаемом формате
func (p *ProgressBar) GetSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	} else if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	} else if bytes < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(bytes)/1024/1024)
	}
	return fmt.Sprintf("%.2f GB", float64(bytes)/1024/1024/1024)
}

// GetETA возвращает оценочное время завершения
func (p *ProgressBar) GetETA() string {
	if p.speed == 0 {
		return "вычисляется..."
	}

	remaining := p.total - p.current
	if remaining <= 0 {
		return "завершено"
	}

	seconds := float64(remaining) / 1024 / p.speed
	if seconds < 60 {
		return fmt.Sprintf("%.0f сек", seconds)
	} else if seconds < 3600 {
		return fmt.Sprintf("%.0f мин %.0f сек", seconds/60, seconds/60)
	}
	return fmt.Sprintf("%.0f ч %.0f мин", seconds/3600, seconds/3600)
}

// String возвращает красивое отображение прогресса
func (p *ProgressBar) String() string {
	percent := p.GetPercentage()
	bar := p.GetProgressBar(20)
	speed := p.GetSpeed()
	downloaded := p.GetSize(p.current)
	total := p.GetSize(p.total)
	eta := p.GetETA()

	return fmt.Sprintf(
		"┌─────────────────────────────┐\n"+
			"│ %s │ %.1f%% │\n"+
			"├─────────────────────────────┤\n"+
			"│ ⬇️  %s / %s                  │\n"+
			"│ ⚡ %s │ ⏱️ %s       │\n"+
			"└─────────────────────────────┘",
		bar, percent,
		downloaded, total,
		speed, eta,
	)
}
