# ARGUS — Agent Release Graduate for Ubuntu Systems

ARGUS monitors Ubuntu image build pipelines via proactive Temporal cron workflows
and a reactive Mattermost bot. See [DESIGN.md](DESIGN.md) for architecture, data types,
and demo flows.

## Environment variables
MATTERMOST_WEBHOOK_URL # Mattermost incoming webhook URL — optional; unset = stdout simulation
TEST_OBSERVER_URL      # Ubuntu Test Observer API base URL (default: https://tests-api.ubuntu.com)
TEMPORAL_HOST          # Temporal server (default: localhost:7233)
DEFAULT_RELEASE        # Pin status table to a release; empty = auto-detect

# TODO: re-add when log analysis is implemented
# OPENROUTER_API_KEY   # OpenRouter API key
# LLM_MODEL            # OpenRouter model slug

## Key conventions
- WebhookClient and ArtefactClient and LLMClient are interfaces — always have a mock/stub impl for tests
- Errors wrapped with context: fmt.Errorf("activityName: %w", err)
- snapshot.json written atomically (write to tmp file, rename)
- Never import internal packages circularly — config has no deps, llm depends only on config
- mattermost.Dispatch is pure (no I/O side effects beyond the WebhookClient) — easy to unit test

## Running locally

### Option A — Docker Compose (recommended)
```
cp .env.example .env          # no required vars; optionally set MATTERMOST_WEBHOOK_URL
make up                       # builds images, starts temporal + bot
make down                     # tear down (state volume is preserved)
```
Services exposed:
- http://localhost:8233  — Temporal Web UI

Note: in Docker Compose the bot runs in a container. For interactive REPL use Option B.

### Option B — 2 terminals (no Docker)
```
# terminal 1
temporal server start-dev

# terminal 2
make run-bot   # or: go run ./cmd/bot/
```
Type commands at the `you>` prompt. No API keys required for local development.

## Makefile targets
| Target        | What it does                                  |
|---------------|-----------------------------------------------|
| `make build`  | Compile bot binary into `bin/`                |
| `make clean`  | Remove `bin/`                                 |
| `make test`   | `go test -race -count=1 ./...`                |
| `make lint`   | `golangci-lint run ./...`                     |
| `make check`  | lint + test (pre-commit gate)                 |
| `make run-bot`| `go run ./cmd/bot/`                           |
| `make up`     | `docker compose up --build -d`               |
| `make down`   | `docker compose down`                         |

## Test strategy
Test these:   state/snapshot.go (diff logic), analyze_log.go (JSON parsing), mattermost/dispatch.go (command routing)
Skip today:   Temporal workflow sequencing, real HTTP clients

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
- Use the imperative mood, present tense: "add dispatch handler" not "added"
- Subject line ≤ 72 characters
- Format: `<type>: <subject>` where type is one of:
  - `feat` — new feature or behaviour
  - `fix` — bug fix
  - `test` — adding or fixing tests
  - `refactor` — restructuring without behaviour change
  - `chore` — tooling, deps, CI, config
  - `docs` — documentation only
- One logical change per commit; don't bundle unrelated changes
- Reference block number when completing a build-order block: e.g. `feat(block-2): add mattermost dispatch package`
