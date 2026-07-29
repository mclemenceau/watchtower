// Package application provides the protocol-agnostic command routing layer.
// It imports only domain and ports — never any adapter package.
package application

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/mclemenceau/watchtower/internal/domain"
	"github.com/mclemenceau/watchtower/internal/intent"
	"github.com/mclemenceau/watchtower/internal/ports"
)

// Dispatch routes an incoming message to the appropriate handler and sends the
// reply via notifier. artefacts is the current snapshot (may be nil/empty on first boot).
// failures is the current FailureStore (may be empty on first boot).
// Each artefact's Builds field is populated by the cron workflow and contains
// cached test execution data used by the `tests` commands.
//
// keyword is the optional trigger prefix (e.g. "@watchtower"). When set, messages
// that do NOT start with the keyword are silently ignored, and the keyword is
// stripped before routing. Pass an empty string to disable keyword filtering
// (every message is dispatched).
//
// resolver is an optional LLM-backed intent resolver. When non-nil and a message
// does not match any keyword pattern, Dispatch delegates to the resolver to
// interpret free-text and either re-dispatch the resolved command or ask a
// clarifying question. sessionID identifies the conversation (e.g. "repl" or
// a channel+user composite) for multi-turn clarification. Pass a nil resolver
// to keep the original "I didn't understand" behaviour.
//
// logFetcher, llm, and launchpad are optional dependencies required only by the
// `investigate` command. Pass nil to disable investigation (the command returns
// a helpful error). launchpad enables two-hop Launchpad librarian log resolution;
// when nil the command falls back to the cd-build-log.
//
// triggerAnalysis is an optional callback invoked when the user requests
// `analyse failures [release]`. It should start a background FailureAnalysisWorkflow.
// Pass nil to disable on-demand analysis triggering.
//
// summaryForProducts is an optional allow-list of product/OS names (case-insensitive).
// When non-empty, artefacts whose OS is not in the list are excluded from all
// summary and detail views. Pass nil to include all products.
//
// releasesScope is the ordered release scope used by the `summary` command and fetch
// filtering (mirrors the WATCHTOWER_RELEASES_SCOPE env var). Pass nil to include all releases.
func Dispatch(
	ctx context.Context,
	sessionID, msg string,
	artefacts []domain.Artefact,
	failures domain.FailureStore,
	releasesScope []string,
	summaryForProducts []string,
	notifier ports.Notifier,
	keyword string,
	resolver *intent.Resolver,
	logFetcher ports.LogFetcher,
	llm ports.LLMClient,
	launchpad ports.LaunchpadSource,
	triggerAnalysis func(release string) error,
) error {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return nil
	}

	// Apply the env-level product allow-list: filter out artefacts whose OS is
	// not in the configured products list. This narrows all summary and detail
	// views without affecting per-command product filters supplied by the user.
	if len(summaryForProducts) > 0 {
		var filtered []domain.Artefact
		for _, art := range artefacts {
			if summaryProductAllowed(summaryForProducts, art.OS) {
				filtered = append(filtered, art)
			}
		}
		artefacts = filtered
	}

	// Apply the env-level releases scope: filter out artefacts whose Release is
	// not in the configured scope. This ensures all commands (builds status,
	// tests status, failures, summary, investigate) only operate on the scoped
	// releases, even when the snapshot on disk contains more releases.
	if len(releasesScope) > 0 {
		var filtered []domain.Artefact
		for _, art := range artefacts {
			if releaseAllowed(releasesScope, art.Release) {
				filtered = append(filtered, art)
			}
		}
		artefacts = filtered
	}

	// Keyword filtering: if a keyword is configured, only process messages that
	// start with it (case-insensitive), then strip the prefix.
	if kw := strings.ToLower(strings.TrimSpace(keyword)); kw != "" {
		lower := strings.ToLower(msg)
		if !strings.HasPrefix(lower, kw) {
			return nil // not addressed to us
		}
		msg = strings.TrimSpace(msg[len(kw):])
		if msg == "" {
			msg = "greet" // bare keyword → friendly greeting
		}
	}

	lower := strings.ToLower(msg)
	parts := strings.Fields(msg)

	switch {
	case lower == "greet":
		return notifier.Send(GreetText())

	case lower == "help":
		return notifier.Send(HelpText())

	case lower == "summary":
		return notifier.Send(FormatScheduledSummary(artefacts, releasesScope))

	case lower == "builds status":
		return notifier.Send(FormatBuildsStatusSummary(artefacts))

	case strings.HasPrefix(lower, "builds status ") && len(parts) == 3:
		return notifier.Send(FormatBuildsStatusRelease(artefacts, parts[2], ""))

	case strings.HasPrefix(lower, "builds status ") && len(parts) == 4:
		return notifier.Send(FormatBuildsStatusRelease(artefacts, parts[2], parts[3]))

	case lower == "builds" || (strings.HasPrefix(lower, "builds") && len(parts) == 2):
		return notifier.Send("Usage: `builds status` · `builds status <release>` · `builds status <release> <product>`")

	case lower == "tests status":
		return notifier.Send(FormatTestsStatusSummary(artefacts))

	case strings.HasPrefix(lower, "tests status ") && len(parts) == 3:
		return notifier.Send(FormatTestsStatusRelease(artefacts, parts[2], ""))

	case strings.HasPrefix(lower, "tests status ") && len(parts) == 4:
		return notifier.Send(FormatTestsStatusRelease(artefacts, parts[2], parts[3]))

	case lower == "tests" || (strings.HasPrefix(lower, "tests") && len(parts) == 2):
		return notifier.Send("Usage: `tests status` · `tests status <release>` · `tests status <release> <product>`")

	case strings.HasPrefix(lower, "investigate ") && len(parts) == 2:
		return investigateArtefact(ctx, parts[1], artefacts, logFetcher, llm, launchpad, notifier)

	case lower == "investigate":
		return notifier.Send("Usage: `investigate <artefact-id>` — use `builds status <release>` to find IDs")

	case lower == "failures":
		return notifier.Send(FormatFailuresSummary(failures.ActiveFailures("", ""), "", ""))

	case strings.HasPrefix(lower, "failures ") && len(parts) == 2:
		return notifier.Send(FormatFailuresSummary(failures.ActiveFailures(parts[1], ""), parts[1], ""))

	case strings.HasPrefix(lower, "failures ") && len(parts) == 3:
		return notifier.Send(FormatFailuresSummary(failures.ActiveFailures(parts[1], parts[2]), parts[1], parts[2]))

	case strings.HasPrefix(lower, "failure detail ") && len(parts) == 3:
		return failureDetailCommand(parts[2], failures, notifier)

	case lower == "failure detail":
		return notifier.Send("Usage: `failure detail <artefact-id>` — use `failures` to find IDs")

	case lower == "analyse failures" || lower == "analyze failures":
		return triggerAnalysisCommand(ctx, "", triggerAnalysis, notifier)

	case (strings.HasPrefix(lower, "analyse failures ") || strings.HasPrefix(lower, "analyze failures ")) && len(parts) == 3:
		return triggerAnalysisCommand(ctx, parts[2], triggerAnalysis, notifier)

	default:
		if resolver != nil {
			// Send an immediate acknowledgement so the user knows the bot is working.
			// Free-text intent resolution involves an LLM call that can take several seconds.
			_ = notifier.Send("_thinking…_")
			// Build a filtered state snapshot to give the LLM relevant context.
			// State is re-serialised on every call so answers always reflect the
			// latest snapshot (important for multi-turn conversations).
			contextJSON := BuildContext(msg, artefacts, failures)
			res := resolver.Resolve(ctx, sessionID, msg, contextJSON)
			switch res.Kind {
			case intent.Dispatched:
				// Re-dispatch with the resolved command; no further intent resolution.
				// Pass logFetcher/llm/launchpad so that `investigate` is reachable via
				// free-form (e.g. "what's wrong with artefact 1234?").
				return Dispatch(ctx, sessionID, res.Command, artefacts, failures, releasesScope, summaryForProducts, notifier, "", nil, logFetcher, llm, launchpad, triggerAnalysis)
			case intent.Answered:
				// LLM answered directly using the state data — send the prose reply.
				return notifier.Send(res.Reply)
			case intent.NeedsInfo:
				return notifier.Send(res.Reply)
			case intent.Failed:
				return notifier.Send(res.Reply)
			}
		}
		return notifier.Send(fmt.Sprintf("I didn't quite get `%s` — try asking me in natural language, or type `help` to see all commands.", msg))
	}
}

// investigateArtefact looks up the artefact by ID, resolves the best available
// log (cd-build-log or Launchpad librarian via two-hop resolution), runs LLM
// root-cause analysis, and sends a formatted investigation report.
func investigateArtefact(
	ctx context.Context,
	idStr string,
	artefacts []domain.Artefact,
	logFetcher ports.LogFetcher,
	llm ports.LLMClient,
	launchpad ports.LaunchpadSource,
	notifier ports.Notifier,
) error {
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return notifier.Send(fmt.Sprintf("Invalid artefact ID `%s` — must be a number. Use `builds status <release>` to find IDs.", idStr))
	}

	var art *domain.Artefact
	for i := range artefacts {
		if artefacts[i].ID == id {
			art = &artefacts[i]
			break
		}
	}
	if art == nil {
		return notifier.Send(fmt.Sprintf("Artefact ID `%d` not found in snapshot. Use `builds status <release>` to list available IDs.", id))
	}

	if logFetcher == nil || llm == nil {
		return notifier.Send("Investigation requires an LLM — set OPENROUTER_API_KEY to enable.")
	}

	if domain.LogURLFromImageURL(art.ImageURL) == "" {
		return notifier.Send(fmt.Sprintf("No log URL available for artefact **%s** (ID: %d) — image URL is missing or unrecognised.", art.Name, art.ID))
	}

	_ = notifier.Send(fmt.Sprintf("Fetching and analysing log for **%s** (ID: %d)…", art.Name, art.ID))

	analysis, source, err := analyzeLog(ctx, *art, logFetcher, launchpad, llm)
	if err != nil {
		return notifier.Send(fmt.Sprintf("Investigation failed for **%s**: %s", art.Name, err.Error()))
	}

	return notifier.Send(FormatInvestigation(*art, analysis, source))
}

// summaryProductAllowed reports whether product (case-insensitive) is in the allow-list.
func summaryProductAllowed(list []string, product string) bool {
	for _, p := range list {
		if strings.EqualFold(p, product) {
			return true
		}
	}
	return false
}

// releaseAllowed reports whether release (case-insensitive) is in the scope list.
func releaseAllowed(scope []string, release string) bool {
	for _, r := range scope {
		if strings.EqualFold(r, release) {
			return true
		}
	}
	return false
}

// triggerAnalysisCommand triggers a background failure analysis workflow.
// release may be empty to analyse all pending failures.
func triggerAnalysisCommand(_ context.Context, release string, trigger func(string) error, notifier ports.Notifier) error {
	if trigger == nil {
		return notifier.Send("On-demand failure analysis is not configured (LLM or Temporal unavailable).")
	}
	if err := trigger(release); err != nil {
		return notifier.Send(fmt.Sprintf("Failed to start failure analysis: %s", err.Error()))
	}
	if release != "" {
		return notifier.Send(fmt.Sprintf("Failure analysis started for **%s** — results will appear in `failures %s` once complete.", release, release))
	}
	return notifier.Send("Failure analysis started — results will appear in `failures` once complete.")
}

// failureDetailCommand looks up a FailureRecord by artefact ID and returns its
// full detail including any LLM analysis. Only active (unresolved) records are
// searched — consistent with the `failures` command.
func failureDetailCommand(idStr string, failures domain.FailureStore, notifier ports.Notifier) error {
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return notifier.Send(fmt.Sprintf(
			"Invalid artefact ID `%s` — must be a number. Use `failures` to find IDs.",
			idStr,
		))
	}
	for _, rec := range failures.ActiveFailures("", "") {
		if rec.ArtefactID == id {
			return notifier.Send(FormatFailureDetail(rec))
		}
	}
	return notifier.Send(fmt.Sprintf(
		"No active failure found for artefact ID `%d`. Use `failures` to list active IDs.",
		id,
	))
}
