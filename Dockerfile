FROM golang:1.21-alpine

# Устанавливаем ffmpeg и yt-dlp
RUN apk add --no-cache \
    ffmpeg \
    yt-dlp \
    bash \
    curl

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