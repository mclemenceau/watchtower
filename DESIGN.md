# Watchtower — Design Reference

## What this is

An AI-powered release monitoring agent for Ubuntu image build pipelines.
Watchtower runs two concurrent modes:

**Proactive (Temporal cron workflow — no human trigger):**
- Every 10 min: fetch artefacts → diff against local snapshot → post change report to Mattermost if anything changed

**Reactive (human-triggered via Mattermost channel):**
- Keyword-based command dispatch — no LLM required for standard queries
- Reads from the same local snapshot maintained by the cron workflow

## Tech stack

| Component | Technology |
|-----------|------------|
| Language | Go 1.21+ |
| Workflow orchestration | Temporal (`temporalio/auto-setup`) |
| Pipeline data | Ubuntu Test Observer API (`https://tests-api.ubuntu.com`) |
| Mattermost I/O | Incoming webhooks (real) / stdout simulation (dev) |
| State | `state/snapshot.json` — atomic write (tmp → rename), no database |



## Architecture

Watchtower uses a **hexagonal (ports & adapters)** architecture. The dependency
direction flows strictly inward:

```
cmd/bot → adapters → application → ports → domain
cmd/bot → adapters → ports
activities → ports
workflow → application → domain
intent → ports (LLMClient)
```

### Domain layer — `internal/domain/`

Pure types and business logic with **zero I/O and zero internal imports**.
All core types live here:

- `Artefact`, `ArtefactBuild`, `TestExecution`, `Environment`
- `ChangeReport`, `ArtefactDelta`, `LogAnalysis`

Pure helper functions: `IsBuiltToday`, `BuildStatus`, `ImageAge`,
`LogURLFromImageURL`, `LogCell`, `IsDisplayable`, `ExecStatusEmoji`.

### Ports layer — `internal/ports/`

Go interfaces defining what the application needs from the outside world:

| Interface | Purpose |
|-----------|---------|
| `ArtefactSource` | Fetch Ubuntu image artefacts |
| `BuildSource` | Fetch build/test execution data per artefact |
| `Notifier` | Send messages to a channel (Mattermost, stdout…) |
| `SnapshotStore` | Persist and retrieve the artefact snapshot |
| `LLMClient` | Call a large language model |
| `LogFetcher` | Retrieve build log content from a URL |

### Application layer — `internal/application/`

Protocol-agnostic command routing and response formatting. Imports only `domain`
and `ports` — **never any adapter package**.

- `commands.go` — `Dispatch(ctx, sessionID, msg, artefacts, defaultRelease, notifier, keyword, resolver, logFetcher, llm)`: routes messages to handlers
- `formatters.go` — pure `string`-returning functions: `FormatBuildsStatusSummary`, `FormatBuildsStatusRelease`, `FormatTestsStatusSummary`, `FormatTestsStatusRelease`, `FormatChangeReport`, `FormatInvestigation`, `HelpText`
- `loganalysis.go` — `analyzeLog` helper: fetches log via `LogFetcher`, calls LLM, returns `domain.LogAnalysis`

### Adapters layer — `internal/adapters/`

Concrete implementations of ports. Each sub-package targets one external system:

| Package | Implements | Notes |
|---------|-----------|-------|
| `adapters/testobserver/` | `ArtefactSource`, `BuildSource` | HTTP + Mock |
| `adapters/mattermost/` | `Notifier` + RunREPL/RunPoller | HTTP + Stdout |
| `adapters/openrouter/` | `LLMClient` | HTTP + Mock |

**Adding a new messaging protocol** (e.g. Matrix): implement `ports.Notifier`
and a runner (analogous to `RunPoller`) in `internal/adapters/matrix/`; wire in
`cmd/bot/main.go`. No other files change.

**Adding a new data source**: implement `ports.ArtefactSource` (and optionally
`ports.BuildSource`) in `internal/adapters/<provider>/`; wire in `cmd/bot/main.go`.

### Infrastructure

| Package | Role |
|---------|------|
| `internal/state/` | Atomic JSON snapshot persistence; `Diff` and `LatestRelease` helpers; implements `ports.SnapshotStore` |
| `internal/activities/` | Temporal activity functions; struct fields are port interfaces |
| `internal/workflow/` | Temporal cron workflow; calls `application.FormatChangeReport` |
| `internal/intent/` | LLM-assisted intent resolver for free-text commands |
| `internal/config/` | Env var loading with defaults |

## Project structure

```
cmd/
  bot/
    main.go            Sole wiring point: instantiate adapters, inject via ports
    logger.go          Temporal SDK logger shim
internal/
  domain/
    artefact.go        All core types + pure helper functions
    artefact_test.go   Unit tests for domain helpers
  ports/
    ports.go           ArtefactSource, BuildSource, Notifier, SnapshotStore,
                       LLMClient, LogFetcher interfaces
    ports_test.go      Compile-time interface satisfaction assertions
  application/
    commands.go        Dispatch — protocol-agnostic command router
    formatters.go      FormatBuildsStatus*, FormatTestsStatus*, FormatChangeReport,
                       FormatInvestigation, HelpText — pure string functions
    loganalysis.go     analyzeLog — fetch log + LLM root-cause analysis helper
    commands_test.go   All dispatch and keyword routing tests
    formatters_test.go Unit tests for all formatting functions
  adapters/
    testobserver/
      artefact_client.go  HTTPArtefactSource, HTTPBuildSource
      mock.go             MockArtefactSource, MockBuildSource
    mattermost/
      webhook.go          StdoutNotifier, HTTPNotifier
      repl.go             RunREPL
      poller.go           RunPoller
      poller_test.go      Integration tests for RunPoller
    openrouter/
      client.go           OpenRouterClient, MockLLMClient
  state/
    snapshot.go        Atomic JSON read/write; Diff; LatestRelease; SnapshotStore impl
    snapshot_test.go   State logic tests
  activities/
    build_status.go    FetchBuildStatus, FetchTestExecutions, LoadSnapshot,
                       SaveSnapshot, FormatStatusTable, NotifyChannel
    analyze_log.go     AnalyzeLog (LLM log root-cause analysis)
    fetch_log.go       FetchLog — delegates to ports.LogFetcher
    analyze_log_test.go
  workflow/
    change_watch.go    10-min cron: fetch → diff → notify if changed
  intent/
    resolver.go        LLM-backed free-text intent resolver
    resolver_test.go
  config/
    config.go          Env var loading with defaults
```

## Core data types

All types live in `internal/domain/`. See `domain/artefact.go` for the full
definitions. Key types:

```go
// Artefact mirrors the Test Observer API response for the image family.
type Artefact struct {
    ID       int    `json:"id"`
    Name     string `json:"name"`
    Version  string `json:"version"` // YYYYMMDD or YYYYMMDD.N (respin)
    OS       string `json:"os"`      // product / Ubuntu variant
    Release  string `json:"release"` // e.g. "noble", "plucky"
    Stage    string `json:"stage"`   // pending | current
    Status   string `json:"status"`  // APPROVED | MARKED_AS_FAILED | UNDECIDED
    Archived bool   `json:"archived"`
    ImageURL string `json:"image_url"`
    Builds   []ArtefactBuild `json:"builds,omitempty"`
}

type ChangeReport struct {
    NewFailures  []ArtefactDelta `json:"new_failures"`
    Recoveries   []ArtefactDelta `json:"recoveries"`
    OtherChanges []ArtefactDelta `json:"other_changes"`
    NewArtefacts []Artefact      `json:"new_artefacts"`
}

type ArtefactDelta struct {
    Name      string `json:"name"`
    Release   string `json:"release"`
    Version   string `json:"version"`
    OldStatus string `json:"old_status"`
    NewStatus string `json:"new_status"`
}
```

## Artefact lifecycle

An artefact's presence in Test Observer is the signal that a build succeeded:

- **Present with today's version** (`YYYYMMDD` or `YYYYMMDD.N`) — build completed; image is available for testing.
- **Present with an older version** — today's build has not landed yet. The cause is indistinguishable from Watchtower's perspective: the build may not have started, may be in progress, or may have failed at the pipeline level before reaching Test Observer.
- **Absent entirely** — the artefact has never been seen, or is no longer tracked by Test Observer.

The `Status` field (`APPROVED` / `UNDECIDED` / `MARKED_AS_FAILED`) is the **test review state** set by humans after testing. It is orthogonal to build availability and is not used in the status table.

## Workflow data flow

### ChangeWatchWorkflow (every 10 min)
```
FetchBuildStatus → FetchTestExecutions → LoadSnapshot → Diff → SaveSnapshot
                                                              └─ if changes → NotifyChannel (markdown change report)
```

## Mattermost interaction model

### Reactive (user-triggered)

| Command | Response |
|---------|----------|
| `builds status` | Build summary for all releases with progress bar |
| `builds status <release>` | Detailed build status for a specific release (includes artefact IDs) |
| `builds status <release> <product>` | Filter detail view to a single product |
| `tests status` | Test summary for all releases with progress bar |
| `tests status <release>` | Detailed test status for a specific release |
| `tests status <release> <product>` | Filter test detail view to a single product |
| `investigate <artefact-id>` | Fetch build log and run LLM root-cause analysis (requires `OPENROUTER_API_KEY`) |
| `help` | Available commands |
| *(anything else)* | LLM intent resolution (if API key set) or "I didn't understand…" |

### Proactive (automatic)

Change reports are posted to the channel whenever the 10-min cron detects:
- New failures (`MARKED_AS_FAILED`)
- Recoveries (`APPROVED` after failure)
- Status changes
- New artefacts

Format: emoji-prefixed sections (`🔴 New Failures`, `🟢 Recoveries`, `🔵 Other Changes`, `🆕 New Artefacts`).

## Terminal simulation (development)

Run `make run-bot` (no Mattermost credentials needed):

```
$ make run-bot
[Watchtower] Bot started. Type a message (Ctrl-D to quit):
you> help
[Watchtower →]
**Watchtower — available commands:** ...

you> builds status
[Watchtower →]
**Build Status** · 2026-06-24 14:00 UTC
...

you> builds status noble
[Watchtower →]
**Build Status** · noble · 2026-06-24 14:00 UTC
...
```

Proactive change reports from the cron workflow print inline with the same
`[Watchtower →]` prefix.

When `MATTERMOST_WEBHOOK_URL` is set, `StdoutNotifier` is replaced by
`HTTPNotifier` with no other code changes.

## Key design decisions

**Hexagonal architecture** — domain, ports, application, and adapters layers
ensure that swapping Mattermost for Matrix (or any protocol) only requires
writing a new adapter. The application layer is untouched.

**Snapshot as query source** — the REPL dispatcher reads `state/snapshot.json`
(maintained by `ChangeWatchWorkflow`) rather than hitting the API on every
command. This keeps latency low and ensures commands are consistent with what
the cron is monitoring.

**Keyword dispatch, not LLM** — all standard commands are handled
deterministically. This makes responses instant, reproducible, and free of
API cost. An LLM is reserved for log analysis (TODO) and intent resolution
for free-text queries.

**Single binary** — `cmd/bot` embeds both the Temporal worker and the REPL
loop. No separate server process is needed.

**No database** — `state/snapshot.json` written atomically (write to `.tmp`,
rename) is sufficient for the monitoring use case.

**Interface-driven testing** — `ArtefactSource`, `BuildSource`, `LLMClient`,
and `Notifier` are all interfaces with mock implementations in their adapter
packages, enabling unit tests without real API calls.
