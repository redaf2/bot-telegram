.PHONY: run build clean

run:
	go run cmd/bot/main.go

build:
	go build -o bin/audiobot cmd/bot/main.go

clean:
	rm -rf temp/* bin/