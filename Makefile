.PHONY: run build test vet lint fmt setup-sheet docker-up docker-down

run:
	go run ./cmd/bot

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

lint: vet

fmt:
	gofmt -w cmd internal

setup-sheet:
	go run ./cmd/setup

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down
