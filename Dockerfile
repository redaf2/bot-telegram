FROM golang:1.21-alpine

# Добавляем репозиторий, где есть yt-dlp
RUN echo "https://dl-cdn.alpinelinux.org/alpine/edge/testing" >> /etc/apk/repositories

# Устанавливаем ffmpeg и yt-dlp через apk
RUN apk update && apk add --no-cache \
    ffmpeg \
    yt-dlp

WORKDIR /app

# Копируем зависимости
COPY go.mod go.sum ./
RUN go mod download

# Копируем код
COPY . .

# Собираем бота
RUN go build -o bot cmd/bot/main.go

# Создаём папку для временных файлов
RUN mkdir -p temp

# Проверяем, что yt-dlp установлен
RUN yt-dlp --version

# Запускаем бота
CMD ["./bot"]