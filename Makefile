.PHONY: build clean test lint check run-bot up down restart-bot reset clean-state

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

## ── Local dev (Option B — 2 terminals, recommended for active development) ──
##
##   terminal 1:  temporal server start-dev
##   terminal 2:  make run-bot
##
## To restart after a code change: Ctrl-C in terminal 2, then make run-bot again.

run-bot:
	go run ./cmd/bot/

clean-state:
	rm -f state/snapshot.json

## ── Docker Compose (Option A — demo / staging) ─────────────────────────────

up:
	docker compose up --build -d

down:
	docker compose down

## Rebuild + restart only the bot container (Temporal/Postgres keep running).
## Use this after a code change when running Option A.
restart-bot:
	docker compose up --build -d bot

## Full reset: stop everything, wipe all volumes (Temporal state + snapshot),
## then bring the whole stack back up from scratch.
reset:
	docker compose down -v
	docker compose up --build -d
