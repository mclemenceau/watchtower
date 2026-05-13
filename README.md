# ARGUS — Agent Release Graduate for Ubuntu Systems

An AI-powered monitoring agent for Ubuntu image build pipelines. ARGUS watches
build status across all active releases, surfaces changes automatically, and
answers natural-language questions about the pipeline state.

---

## What it does

**Proactive monitoring** — without any human input, ARGUS:
- Checks for pipeline changes every 10 minutes and posts a change report to the chat when something shifts
- Publishes a full build status table every 6 hours

**Reactive Q&A** — ask anything in plain English:
- *"What's the current status of all noble builds?"*
- *"Are there any active failures right now?"*
- *"Why did ubuntu-server-amd64 fail?"*

The LLM receives the full live snapshot and produces a direct, formatted answer —
a prose summary for single-image queries, a sorted markdown table for overviews.

---

## Quick start

**Prerequisites:** Docker + Docker Compose, an [OpenRouter](https://openrouter.ai) API key.

```bash
cp .env.example .env          # add your OPENROUTER_API_KEY
make up                       # build images and start the stack
```

| URL | What |
|-----|------|
| http://localhost:8080 | ARGUS chat UI |
| http://localhost:8233 | Temporal workflow dashboard |

Allow ~60 seconds on first start for Temporal to complete its schema migrations.
See [QUICKSTART.md](QUICKSTART.md) for full details including how to manually trigger workflows.

```bash
make down                     # stop (state is preserved across restarts)
```

---

## Architecture

```
                    ┌─────────────────────────────────┐
                    │         Temporal Worker          │
                    │                                  │
                    │  ChangeWatchWorkflow (10 min)    │
                    │    └─ diff snapshot → feed push  │
                    │                                  │
                    │  StatusTableWorkflow (6 h)       │
                    │    └─ full table → feed push     │
                    │                                  │
                    │  QueryWorkflow (on demand)       │
                    │    └─ snapshot + query → LLM     │
                    └──────────────┬──────────────────┘
                                   │ /internal/push
                    ┌──────────────▼──────────────────┐
                    │         Gin HTTP Server          │
                    │  POST /query  GET /feed (SSE)    │
                    └──────────────┬──────────────────┘
                                   │
                    ┌──────────────▼──────────────────┐
                    │           Web UI                 │
                    │  Single-page chat — pipeline     │
                    │  updates + Q&A in one thread     │
                    └─────────────────────────────────┘
```

The worker maintains a local `state/snapshot.json` updated by every
`ChangeWatchWorkflow` run. Query workflows read that snapshot (a fast local
file read) and pass the full artefact table to the LLM in a single call,
so queries are fast and the model has complete context.

---

## Tech stack

| Component | Technology |
|-----------|------------|
| Language | Go 1.21+ |
| Workflow orchestration | [Temporal](https://temporal.io) |
| LLM | [OpenRouter](https://openrouter.ai) (configurable model) |
| Pipeline data | [Ubuntu Test Observer API](https://tests-api.ubuntu.com) |
| Web server | [Gin](https://github.com/gin-gonic/gin) |
| UI | Vanilla HTML/JS + [Vanilla Framework](https://vanillaframework.io) |
| State | Atomic JSON file — no database |

---

## Configuration

Copy `.env.example` to `.env` and set your key:

| Variable | Default | Description |
|----------|---------|-------------|
| `OPENROUTER_API_KEY` | — | **Required.** OpenRouter API key |
| `LLM_MODEL` | `anthropic/claude-sonnet-4-5` | Any model available on OpenRouter |
| `DEFAULT_RELEASE` | auto-detect | Pin the status table to a specific Ubuntu release |
| `TEST_OBSERVER_URL` | `https://tests-api.ubuntu.com` | Pipeline data source |
| `PORT` | `8080` | HTTP server port |
| `TEMPORAL_HOST` | `localhost:7233` | Temporal server address |

---

## Project layout

```
cmd/
  server/     Gin HTTP gateway  (/query, /feed SSE, static UI)
  worker/     Temporal worker entrypoint + cron registration
internal/
  workflow/   ChangeWatchWorkflow, StatusTableWorkflow, QueryWorkflow
  activities/ FetchBuildStatus, FormatStatusTable, AnswerQuery, AnalyzeLog, …
  buildapi/   ArtefactClient interface + HTTP implementation
  llm/        LLMClient interface + OpenRouter implementation
  state/      Atomic snapshot read/write and diff logic
  config/     Environment variable loading
web/
  index.html  Single-page chat UI
```

---

## Development

```bash
make build    # compile worker + server into bin/
make test     # go test -race -count=1 ./...
make lint     # golangci-lint run ./...
make check    # lint + test (pre-commit gate)
```

See [CLAUDE.md](CLAUDE.md) for conventions and [DESIGN.md](DESIGN.md) for
architecture details.
