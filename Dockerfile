FROM golang:1.21-alpine

# Устанавливаем ffmpeg и yt-dlp
RUN apk add --no-cache \
    ffmpeg \
    yt-dlp

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o bot cmd/bot/main.go
RUN mkdir -p temp
RUN yt-dlp --version

CMD ["./bot"]