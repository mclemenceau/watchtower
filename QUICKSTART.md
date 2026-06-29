# ARGUS — Local Dev Quickstart

## Prerequisites

- Go 1.21+
- [Temporal CLI](https://docs.temporal.io/cli) (`temporal`) for Option B
- Docker + Docker Compose for Option A

No API keys are required for local development.

---

## Option B — Two terminals (recommended for active development)

This is the fastest way to iterate. No containers needed.

### Step 1 — Start Temporal

```bash
temporal server start-dev
```

This starts an in-memory Temporal server with the Web UI at
http://localhost:8233.

### Step 2 — Start the bot

```bash
make run-bot
```

ARGUS boots, fetches a fresh snapshot from the Test Observer API, and drops
you into the REPL:

```
[ARGUS] Bot started. Type a message (Ctrl-D to quit):
you>
```

### Step 3 — Try some commands

```
you> help
you> builds status
you> builds status noble
you> tests status
you> tests status plucky
```

The proactive cron workflow runs every 10 minutes in the background. If the
pipeline state changes, a change report prints inline with the same
`[ARGUS →]` prefix.

### Restarting after a code change

```bash
Ctrl-C          # stop the bot
make run-bot    # recompile and restart
```

### Wiping state (force a fresh fetch)

```bash
make clean-state
make run-bot
```

---

## Option A — Docker Compose (demo / staging)

### Step 1 — Configure environment

```bash
cp .env.example .env
```

All variables are optional. To connect a real Mattermost channel, set:

```
MATTERMOST_WEBHOOK_URL=https://your-mattermost-server/hooks/...
MATTERMOST_SERVER_URL=https://your-mattermost-server
MATTERMOST_TOKEN=your-personal-access-token
MATTERMOST_CHANNEL_ID=channel-id-to-monitor
MATTERMOST_KEYWORD=@watchtower   # optional trigger keyword
```

Without these, ARGUS prints all output to stdout inside the container.

### Step 2 — Start the stack

```bash
make up
```

This builds and starts three containers:
- **temporal** — Temporal server with SQLite (`temporalio/auto-setup`)
- **temporal-ui** — Temporal Web UI at http://localhost:8233
- **bot** — ARGUS bot (Temporal worker + Mattermost REPL)

Allow ~60 seconds on first start for Temporal to complete its schema
migrations before the bot connects.

### Step 3 — Attach to the REPL

```bash
docker attach $(docker compose ps -q bot)
```

Detach without stopping with `Ctrl-P Ctrl-Q`.

### Step 4 — Watch logs (optional)

```bash
docker compose logs -f
```

### Stopping

```bash
make down        # stop containers (state volume is preserved)
make reset       # full wipe — removes volumes and restarts fresh
```

---

## Useful Makefile targets

| Target | What it does |
|--------|-------------|
| `make run-bot` | `go run ./cmd/bot/` (Option B) |
| `make build` | Compile bot binary into `bin/` |
| `make test` | `go test -race -count=1 ./...` |
| `make lint` | `golangci-lint run ./...` |
| `make check` | lint + test (pre-commit gate) |
| `make clean-state` | Delete `state/snapshot.json` |
| `make up` | `docker compose up --build -d` |
| `make down` | `docker compose down` (keeps volumes) |
| `make restart-bot` | Rebuild + restart only the bot container |
| `make reset` | Full wipe — `down -v` + `up` |

---

## Manually triggering a workflow

Open the Temporal dashboard at http://localhost:8233, navigate to
**Workflows → Start Workflow**, and fill in:

| Field | Value |
|---|---|
| Task Queue | `argus` |
| Workflow Type | `ChangeWatchWorkflow` |

This forces an immediate fetch → diff → notify cycle without waiting for the
10-minute cron timer.

---

## Connecting a real Mattermost channel

1. In Mattermost, create an **Incoming Webhook** integration and copy the URL
   into `MATTERMOST_WEBHOOK_URL`. ARGUS will post change reports there.

2. To enable the **command bot** (so the channel can query ARGUS):
   - Create a bot account or use a personal access token
   - Set `MATTERMOST_SERVER_URL`, `MATTERMOST_TOKEN`, `MATTERMOST_CHANNEL_ID`
   - Optionally set `MATTERMOST_KEYWORD` (e.g. `@watchtower`) so ARGUS only
     responds to messages that mention that keyword

3. Restart the bot. The poller will begin polling the channel every 15 seconds
   and dispatching any matching commands.
