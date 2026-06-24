.PHONY: build clean test lint check run-bot up down

BOT = bin/bot

## ── Local build ────────────────────────────────────────────────────────────

build: $(BOT)

$(BOT):
	go build -o $@ ./cmd/bot/

clean:
	rm -rf bin/

## ── Quality gates (must pass before every commit) ──────────────────────────

test:
	go test -race -count=1 ./...

lint:
	golangci-lint run ./...

check: lint test

## ── Local dev ───────────────────────────────────────────────────────────────

run-bot:
	go run ./cmd/bot/

## ── Docker Compose ─────────────────────────────────────────────────────────

up:
	docker compose up --build -d

down:
	docker compose down
