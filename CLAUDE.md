# ARGUS — Agent Release Graduate for Ubuntu Systems

## What this is
An AI-powered Graduate Release Engineer agent that monitors Ubuntu image build
pipelines. It has two modes running concurrently:

**Proactive (Temporal cron workflows, no human trigger):**
- Every 6h: fetch all images → render full status table → push to Web UI
- Every 10min: fetch all images → diff vs snapshot → push change summary if anything changed

**Reactive (human-triggered via Web UI chat):**
- Natural language Q&A: "tell me more about ubuntu-desktop-amd64"
- Failure diagnosis: "why isn't ubuntu-server-amd64 building?"
- Fuzzy image name matching via LLM (handles typos and partial names)

## Tech stack
- Language: Go 1.21+
- Orchestration: Temporal (local dev server: `temporal server start-dev`)
- LLM: OpenRouter API (OPENROUTER_API_KEY) — model `anthropic/claude-sonnet-4-5`
- Web UI: single index.html — left panel SSE live feed, right panel chat
- Build status source: internal FastAPI (mock if unavailable)
- Build logs: fetched via HTTP GET from URL in build record; mock with local files
- State: state/snapshot.json — written atomically, no database

## Project structure
cmd/
  server/main.go         # Gin HTTP gateway, /query POST, /feed SSE
  worker/main.go         # Temporal worker entrypoint
internal/
  workflow/
    status_table.go      # 6h cron: fetch all → format table → push to feed
    change_watch.go      # 10min cron: fetch → diff → push if changed
    query.go             # on-demand: intent → fuzzy match → activities → reply
  activities/
    build_status.go      # GET /builds from FastAPI → []Image
    fetch_log.go         # HTTP GET log URL → last 200 lines
    analyze_log.go       # OpenRouter API → root cause JSON
    fuzzy_match.go       # OpenRouter API → match user string to image ID
    compose_reply.go     # OpenRouter API → formatted human reply
  llm/
    openrouter.go        # OpenRouter API client wrapper (interface + real impl)
  buildapi/
    client.go            # BuildClient interface + mock + real HTTP impl
    types.go             # All shared data types
  state/
    snapshot.go          # Atomic read/write of state/snapshot.json
  config/
    config.go            # Env vars with defaults, fail fast if key missing
web/
  index.html             # Two-panel UI: SSE feed left, chat right
mock/
  logs/failed-build.log  # Realistic dpkg failure log for demo

## Core data types (use these exactly)
```go
type Image struct {
    ID         string    `json:"id"`
    Package    string    `json:"package"`
    Series     string    `json:"series"`
    Arch       string    `json:"arch"`
    Status     string    `json:"status"` // BUILDING|SUCCESS|FAILED|CANCELLED
    StartedAt  time.Time `json:"started_at"`
    FinishedAt time.Time `json:"finished_at"`
    LogURL     string    `json:"log_url"`
}

type ChangeReport struct {
    NewFailures  []ImageDelta `json:"new_failures"`
    Recoveries   []ImageDelta `json:"recoveries"`
    OtherChanges []ImageDelta `json:"other_changes"`
    NewImages    []Image      `json:"new_images"`
}

type ImageDelta struct {
    Image     string    `json:"image"`
    OldStatus string    `json:"old_status"`
    NewStatus string    `json:"new_status"`
    Since     time.Time `json:"since"`
}

type AgentReply struct {
    Summary     string   `json:"summary"`
    Category    string   `json:"category"` // infra|code|dependency|flaky|unknown
    Hypothesis  string   `json:"hypothesis"`
    LogExcerpts []string `json:"log_excerpts"`
    NextAction  string   `json:"next_action"`
    WorkflowID  string   `json:"workflow_id"`
}
```

## Environment variables
OPENROUTER_API_KEY     # OpenRouter API key — required, fail fast if missing
BUILD_API_URL          # FastAPI base URL (default: http://localhost:8000)
TEMPORAL_HOST          # Temporal server (default: localhost:7233)
PORT                   # Gateway HTTP port (default: 8080)

## Key conventions
- All LLM calls go through internal/llm/openrouter.go — never call OpenRouter directly from activities
- BuildClient and LLMClient are interfaces — always have a mock impl for tests
- Errors wrapped with context: fmt.Errorf("activityName: %w", err)
- snapshot.json written atomically (write to tmp file, rename)
- Never import internal packages circularly — config has no deps, llm depends only on config

## Running locally

### Option A — Docker Compose (recommended)
```
cp .env.example .env          # fill in OPENROUTER_API_KEY
make up                       # builds images, starts temporal + worker + server
make down                     # tear down (state volume is preserved)
```
Services exposed:
- http://localhost:8080  — Web UI + /query + /feed SSE
- http://localhost:8233  — Temporal Web UI

### Option B — 3 terminals (no Docker)
```
# terminal 1
temporal server start-dev

# terminal 2
export OPENROUTER_API_KEY=...
make run-worker   # or: go run ./cmd/worker/

# terminal 3
export OPENROUTER_API_KEY=...
make run-server   # or: go run ./cmd/server/
```

## Makefile targets
| Target       | What it does                                  |
|--------------|-----------------------------------------------|
| `make build` | Compile worker + server into `bin/`           |
| `make clean` | Remove `bin/`                                 |
| `make test`  | `go test -race -count=1 ./...`                |
| `make lint`  | `golangci-lint run ./...`                     |
| `make check` | lint + test (pre-commit gate)                 |
| `make up`    | `docker compose up --build -d`               |
| `make down`  | `docker compose down`                         |

## Build order (work block by block, confirm each before proceeding)
1. go.mod + config.go + types.go — just types, no logic
2. cmd/worker/main.go — Temporal worker boots, empty workflow registers, visible in localhost:8233
3. internal/buildapi/ — BuildClient interface, mock with 5 mixed-status images
4. internal/state/snapshot.go — atomic JSON read/write, diff logic
5. ChangeWatchWorkflow — polls mock, diffs, logs changes (no LLM yet)
6. internal/llm/openrouter.go — LLMClient interface + real OpenRouter impl
7. Activities one by one, each with _test.go using mocks
8. QueryWorkflow end-to-end
9. cmd/server/main.go — Gin gateway, /query, /feed SSE
10. web/index.html — two-panel UI

## Test strategy
Test these:   state/snapshot.go (diff logic), fuzzy_match.go (JSON parsing), analyze_log.go (JSON parsing)
Skip today:   HTTP handlers, Temporal workflow sequencing, Web UI

## Demo flows
Flow 1 — "What is the status of ubuntu-desktop-amd64?"
  → FetchBuildStatus → ComposeReply → returns status summary

Flow 2 — "Why did ubuntu-server-amd64 fail?"
  → FetchBuildStatus (confirm FAILED) → FetchLog → AnalyzeLog → ComposeReply → returns diagnosis

## Development rules

### Before every commit
ALWAYS run the pre-commit gate and fix all failures before committing:
```
make check   # runs lint then test
```
Or individually:
```
make lint    # golangci-lint run ./...
make test    # go test -race -count=1 ./...
```
Never commit code that fails either check. Install golangci-lint once:
```
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

### CI/CD (GitHub Actions)
Two jobs run on every push and PR to `main` (`.github/workflows/ci.yml`):
- **lint**: golangci-lint via `golangci/golangci-lint-action`
- **test**: `go build ./...` then `go test -race -count=1 ./...`

Both jobs must be green before a branch is merged. Do not merge PRs with failing CI.

### Commit message conventions
- Use the imperative mood, present tense: "add fuzzy match activity" not "added"
- Subject line ≤ 72 characters
- Format: `<type>: <subject>` where type is one of:
  - `feat` — new feature or behaviour
  - `fix` — bug fix
  - `test` — adding or fixing tests
  - `refactor` — restructuring without behaviour change
  - `chore` — tooling, deps, CI, config
  - `docs` — documentation only
- One logical change per commit; don't bundle unrelated changes
- Reference block number when completing a build-order block: e.g. `feat(block-3): add BuildClient interface and mock`
