# ARGUS — Agent Release Graduate for Ubuntu Systems

An automated watchtower for Ubuntu image build pipelines. ARGUS monitors build
and test status across all active releases, posts change alerts to Mattermost
automatically, and answers status queries on demand via channel commands.

---

## What it does

**Proactive monitoring** — without any human input, ARGUS:
- Checks for pipeline changes every 10 minutes and posts a change report to
  the Mattermost channel whenever a failure, recovery, or new artefact appears

**Reactive Q&A** — type commands in the Mattermost channel (or the local REPL):
- `builds status` — build availability table for the latest active release
- `builds status <release>` — builds for a specific release (e.g. `noble`)
- `builds status <release> <product>` — filter by product name
- `tests status` — test execution results for the latest release
- `tests status <release>` — test results for a specific release
- `help` — list all available commands

> **Coming soon:** Natural-language log analysis via LLM — ask *"Why did
> ubuntu-server-amd64 fail?"* and get a root-cause summary from the build log.

---

## Quick start

**No API keys required** for local development.

```bash
# Option B — recommended for development (2 terminals)
temporal server start-dev          # terminal 1
make run-bot                       # terminal 2 — type commands at the you> prompt
```

```bash
# Option A — Docker Compose (demo / staging)
cp .env.example .env               # no required vars; optionally set MATTERMOST_WEBHOOK_URL
make up                            # build images and start the stack
```

| URL | What |
|-----|------|
| http://localhost:8233 | Temporal workflow dashboard |

See [QUICKSTART.md](QUICKSTART.md) for full setup details.

---

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│  cmd/bot  (single binary)                               │
│                                                         │
│  ┌──────────────────────┐   ┌───────────────────────┐  │
│  │   Temporal Worker    │   │  Mattermost Interface │  │
│  │                      │   │                       │  │
│  │  ChangeWatchWorkflow │   │  REPL  (local dev)    │  │
│  │  (every 10 min)      │──▶│  Poller (REST API)    │  │
│  │  fetch → diff        │   │  Webhook (outbound)   │  │
│  │  → notify if changed │   │                       │  │
│  └──────────────────────┘   └───────────┬───────────┘  │
└───────────────────────────────────────── │ ────────────┘
                                           │ keyword dispatch
                                           ▼
                                  state/snapshot.json
                                  (atomic writes, no DB)
```

The cron workflow maintains a local snapshot. The Mattermost interface (REPL
or channel poller) reads from that snapshot to answer commands instantly —
no API call needed per query.

---

## Tech stack

| Component | Technology |
|-----------|------------|
| Language | Go 1.21+ |
| Workflow orchestration | [Temporal](https://temporal.io) |
| Pipeline data | [Ubuntu Test Observer API](https://tests-api.ubuntu.com) |
| Mattermost I/O | Incoming webhooks (real) / stdout simulation (dev) |
| LLM (upcoming) | [OpenRouter](https://openrouter.ai) — log root-cause analysis |
| State | Atomic JSON file — no database |

---

## Configuration

Copy `.env.example` to `.env`. All variables are optional for local development.

| Variable | Default | Description |
|----------|---------|-------------|
| `MATTERMOST_WEBHOOK_URL` | — | Incoming webhook URL — omit to use stdout simulation |
| `MATTERMOST_SERVER_URL` | — | Server URL for channel poller (e.g. `https://chat.example.com`) |
| `MATTERMOST_TOKEN` | — | Personal access token for the poller |
| `MATTERMOST_CHANNEL_ID` | — | Channel ID to poll for incoming commands |
| `MATTERMOST_KEYWORD` | — | Optional trigger keyword (e.g. `@watchtower`) |
| `DEFAULT_RELEASE` | auto-detect | Pin status tables to a specific Ubuntu release |
| `TEST_OBSERVER_URL` | `https://tests-api.ubuntu.com` | Pipeline data source |
| `TEMPORAL_HOST` | `localhost:7233` | Temporal server address |
| `OPENROUTER_API_KEY` | — | (upcoming) OpenRouter key for log analysis |
| `LLM_MODEL` | — | (upcoming) Model slug, e.g. `anthropic/claude-sonnet-4-5` |

---

## Project layout

```
cmd/
  bot/            Single entrypoint: Temporal worker + Mattermost REPL + poller
internal/
  workflow/       ChangeWatchWorkflow (10-min cron)
  activities/     FetchBuildStatus, FetchTestExecutions, FormatStatusTable,
                  NotifyChannel, FetchLog, AnalyzeLog (LLM — upcoming)
  mattermost/     WebhookClient, keyword dispatch, REPL, channel poller
  buildapi/       ArtefactClient interface + HTTP implementation
  testapi/        TestClient interface + HTTP implementation
  llm/            LLMClient interface + OpenRouter implementation (upcoming)
  state/          Atomic snapshot read/write and diff logic
  config/         Environment variable loading
```

---

## Development

```bash
make build    # compile bot binary into bin/
make test     # go test -race -count=1 ./...
make lint     # golangci-lint run ./...
make check    # lint + test (pre-commit gate)
```

See [QUICKSTART.md](QUICKSTART.md) for running locally and [DESIGN.md](DESIGN.md)
for architecture details and data types.
