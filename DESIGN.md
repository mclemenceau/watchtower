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

> **TODO:** Log analysis via LLM (OpenRouter) will be added back in a future block.

## Project structure

```
cmd/
  bot/main.go             Single entrypoint: Temporal worker + Mattermost REPL
internal/
  workflow/
    change_watch.go       10-min cron: fetch → diff snapshot → notify if changed
  activities/
    build_status.go       FetchBuildStatus, LoadSnapshot, SaveSnapshot,
                          FormatStatusTable, NotifyChannel
    analyze_log.go        TODO stub: AnalyzeLog (LLM log root-cause analysis)
    fetch_log.go          FetchLog (GET log URL → last 200 lines)
  mattermost/
    webhook.go            WebhookClient interface, StdoutWebhookClient, HTTPWebhookClient
    dispatch.go           Keyword router: status, builds, releases, help
    repl.go               RunREPL — stdin loop for terminal simulation
  buildapi/
    client.go             ArtefactClient interface + HTTPClient (Test Observer)
    types.go              Shared data types (Artefact, ChangeReport, ArtefactDelta)
  llm/
    openrouter.go         LLMClient interface + OpenRouterClient + MockLLMClient
                          (kept for future log analysis; not wired in production yet)
  state/
    snapshot.go           Atomic JSON read/write; Diff logic; LatestRelease helper
  config/
    config.go             Env var loading with defaults
```

## Core data types

```go
// Artefact mirrors the Test Observer API response for the image family.
type Artefact struct {
    ID       int    `json:"id"`
    Name     string `json:"name"`
    Version  string `json:"version"` // YYYYMMDD or YYYYMMDD.N (respin)
    OS       string `json:"os"`      // product / Ubuntu variant
    Release  string `json:"release"` // e.g. "noble", "oracular"
    Stage    string `json:"stage"`   // pending | current
    Status   string `json:"status"`  // APPROVED | MARKED_AS_FAILED | UNDECIDED
    Archived bool   `json:"archived"`
    ImageURL string `json:"image_url"`
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
FetchBuildStatus → LoadSnapshot → Diff → SaveSnapshot
                                       └─ if changes → NotifyChannel (markdown change report)
```

## Mattermost interaction model

### Reactive (user-triggered)

| Command | Response |
|---------|----------|
| `status` | Status table for the latest (or pinned) release |
| `builds <release>` | All builds for a specific release |
| `releases` | List of all known releases with artefact counts |
| `help` | Available commands |
| *(anything else)* | "I didn't understand…" + pointer to help |

### Proactive (automatic)

Change reports are posted to the channel whenever the 10-min cron detects:
- New failures (`MARKED_AS_FAILED`)
- Recoveries (`APPROVED` after failure)
- Status changes
- New artefacts

Format: emoji-prefixed lines (`🔴 FAILED`, `🟢 APPROVED`, `🔵 CHANGED`, `🆕 NEW`).

## Terminal simulation (development)

Run `make run-bot` (no Mattermost credentials needed):

```
$ make run-bot
[Watchtower] Bot started. Type a message (Ctrl-D to quit):
you> help
[Watchtower →]
**Watchtower — available commands:** ...

you> status
[Watchtower →]
**Build Status — plucky** · 2026-06-24 14:00 UTC
...

you> builds noble
[Watchtower →]
**Builds for noble** (2 artefacts) ...
```

Proactive change reports from the cron workflow print inline with the same
`[Watchtower →]` prefix.

When `MATTERMOST_WEBHOOK_URL` is set, `StdoutWebhookClient` is replaced by
`HTTPWebhookClient` with no other code changes.

## Key design decisions

**Snapshot as query source** — the REPL dispatcher reads `state/snapshot.json`
(maintained by `ChangeWatchWorkflow`) rather than hitting the API on every
command. This keeps latency low and ensures commands are consistent with what
the cron is monitoring.

**Keyword dispatch, not LLM** — all standard commands are handled
deterministically. This makes responses instant, reproducible, and free of
API cost. An LLM is reserved for log analysis (TODO).

**Single binary** — `cmd/bot` embeds both the Temporal worker and the REPL
loop. No separate server process is needed.

**No database** — `state/snapshot.json` written atomically (write to `.tmp`,
rename) is sufficient for the monitoring use case.

**Interface-driven testing** — `ArtefactClient`, `LLMClient`, and
`WebhookClient` are all interfaces with mock/stub implementations, enabling
unit tests without real API calls.
