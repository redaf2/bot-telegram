FROM golang:1.21-alpine

# Устанавливаем ffmpeg, python3, pip и yt-dlp
RUN apk add --no-cache \
    ffmpeg \
    python3 \
    py3-pip \
    && pip3 install --upgrade pip \
    && pip3 install yt-dlp

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o bot cmd/bot/main.go
RUN mkdir -p temp
RUN yt-dlp --version

CMD ["./bot"]