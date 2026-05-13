.PHONY: build clean test lint check run-worker run-server up down

WORKER = bin/worker
SERVER = bin/server

## ── Local build ────────────────────────────────────────────────────────────

build: $(WORKER) $(SERVER)

$(WORKER):
	go build -o $@ ./cmd/worker/

$(SERVER):
	go build -o $@ ./cmd/server/

clean:
	rm -rf bin/

## ── Quality gates (must pass before every commit) ──────────────────────────

test:
	go test -race -count=1 ./...

lint:
	golangci-lint run ./...

check: lint test

## ── Local dev (3 terminals) ────────────────────────────────────────────────

run-worker:
	go run ./cmd/worker/

run-server:
	go run ./cmd/server/

## ── Docker Compose ─────────────────────────────────────────────────────────

up:
	docker compose up --build -d

down:
	docker compose down
