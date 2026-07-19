FROM golang:1.24-alpine

# Добавляем репозиторий, где есть yt-dlp
RUN echo "https://dl-cdn.alpinelinux.org/alpine/edge/testing" >> /etc/apk/repositories

# Устанавливаем ffmpeg, yt-dlp и утилиты
RUN apk update && apk add --no-cache \
    ffmpeg \
    yt-dlp \
    curl

# 🔥 ОБНОВЛЯЕМ yt-dlp ДО ПОСЛЕДНЕЙ ВЕРСИИ (через curl)
RUN curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o /usr/local/bin/yt-dlp && \
    chmod a+rx /usr/local/bin/yt-dlp

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

COPY cookies.txt /app/cookies.txt
COPY hall.wav /app/hall.wav

RUN go build -o bot cmd/bot/main.go
RUN mkdir -p temp

# Проверяем версию (убедимся, что обновилось)
RUN yt-dlp --version

CMD ["./bot"]