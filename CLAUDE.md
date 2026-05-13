# ARGUS — Agent Release Graduate for Ubuntu Systems

ARGUS monitors Ubuntu image build pipelines via proactive Temporal cron workflows
and a reactive Web UI chat. See [DESIGN.md](DESIGN.md) for architecture, data types,
and demo flows.

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

## Test strategy
Test these:   state/snapshot.go (diff logic), fuzzy_match.go (JSON parsing), analyze_log.go (JSON parsing)
Skip today:   HTTP handlers, Temporal workflow sequencing, Web UI

## Development rules

### Keeping docs in sync
When adding a new feature or making a significant design change, update both:
- **DESIGN.md** — project structure, data types, demo flows, tech stack
- **CLAUDE.md** — conventions, env vars, test strategy, if those change

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
