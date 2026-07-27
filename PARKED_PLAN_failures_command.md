# Parked Plan — failures command improvements

Parked after commit a138a07. Load this file to resume.

## Context

The `failures` command family was partially improved (INFRA descriptions stored,
`FormatFailureDetail` written) but the following work is not yet done.

## Remaining tasks

### 1. New command: `failure detail <artefact-id>`

Wire the already-written `FormatFailureDetail` (formatters.go) to a new dispatch route.

Routing to add in `internal/application/commands.go`:
```go
case strings.HasPrefix(lower, "failure detail ") && len(parts) == 3:
    return failureDetailCommand(parts[2], failures, notifier)
case lower == "failure detail":
    return notifier.Send("Usage: `failure detail <artefact-id>` — use `failures` to find IDs")
```

New helper in `commands.go`:
- Parse `idStr` as int; search `failures.ActiveFailures("","")` by `ArtefactID`.
- If found: call `FormatFailureDetail(rec)`.
- If not found: return helpful error message.

### 2. Skip INFRA failures in `PendingAnalysis`

`internal/domain/artefact.go` — `PendingAnalysis` currently returns all
unresolved records with `Analysis == nil`, including INFRA ones. INFRA
records already have a deterministic description; sending them to the LLM
wastes tokens.

Change: add `|| r.FailureKind == BuildFailureKindInfra` to the skip
condition inside `PendingAnalysis`. UNKNOWN records should still be sent
to the LLM (same as PRODUCT) since that constant is reserved but never
set by ParseBuildStatusFromLog today — future-proofing.

### 3. Update `HelpText` and `GreetText`

`internal/application/formatters.go`:
- Add `failure detail <artefact-id>` row to the help table.
- Optionally mention it in `GreetText` under the Failures bullet.

### 4. Add missing tests

#### `internal/application/formatters_test.go`
- `TestFormatFailuresSummary_Empty` — no records → correct "No active failures" string
- `TestFormatFailuresSummary_WithKindNoAnalysis` — shows `INFRA: <desc>` when Analysis==nil
- `TestFormatFailuresSummary_WithAnalysis` — shows `Category: Hypothesis` (overrides kind)
- `TestFormatFailuresSummary_ProductAnalysisPending` — shows `_(analysis pending)_` for PRODUCT with no kind
- `TestFormatFailureDetail_WithKindNoAnalysis` — Kind line rendered, "analysis not yet available"
- `TestFormatFailureDetail_WithAnalysis` — full analysis block rendered

#### `internal/application/commands_test.go`
- `TestDispatchFailures_Empty` — `failures` with empty store → "No active failures"
- `TestDispatchFailures_WithRecords` — `failures` with a record → formatted output
- `TestDispatchFailures_Release` — `failures <release>` filters correctly
- `TestDispatchFailureDetail_Found` — `failure detail <id>` returns detail for known ID
- `TestDispatchFailureDetail_NotFound` — `failure detail <id>` returns error for unknown ID
- `TestDispatchFailureDetail_NoArgs` — bare `failure detail` → usage hint

#### `internal/domain/artefact_test.go`
- `TestPendingAnalysis_SkipsINFRA` — INFRA records not returned by PendingAnalysis
- `TestPendingAnalysis_IncludesPRODUCT` — PRODUCT records are returned

## Decisions already made

- `failure detail` lookup uses only active (unresolved) records — consistent with `failures` command.
- `PendingAnalysis` filters at domain level (Option A), not in `AnalyseFailures` activity.
- UNKNOWN FailureKind is included in LLM analysis (same as PRODUCT).
- `FormatFailureDetail` already written and correct — no changes needed there.
