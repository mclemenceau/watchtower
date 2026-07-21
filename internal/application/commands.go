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
// allowedProducts is an optional allow-list of product/OS names (case-insensitive).
// When non-empty, artefacts whose OS is not in the list are excluded from all
// summary and detail views. Pass nil to include all products.
//
// summaryForReleases is the ordered release list used by the `summary` command
// (mirrors the SUMMARY_FOR_RELEASES env var). Pass nil to include all releases.
func Dispatch(
	ctx context.Context,
	sessionID, msg string,
	artefacts []domain.Artefact,
	defaultRelease string,
	summaryForProducts []string,
	summaryForReleases []string,
	notifier ports.Notifier,
	keyword string,
	resolver *intent.Resolver,
	logFetcher ports.LogFetcher,
	llm ports.LLMClient,
	launchpad ports.LaunchpadSource,
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

	// Keyword filtering: if a keyword is configured, only process messages that
	// start with it (case-insensitive), then strip the prefix.
	if kw := strings.ToLower(strings.TrimSpace(keyword)); kw != "" {
		lower := strings.ToLower(msg)
		if !strings.HasPrefix(lower, kw) {
			return nil // not addressed to us
		}
		msg = strings.TrimSpace(msg[len(kw):])
		if msg == "" {
			msg = "help" // bare keyword → show help
		}
	}

	lower := strings.ToLower(msg)
	parts := strings.Fields(msg)

	switch {
	case lower == "help":
		return notifier.Send(HelpText())

	case lower == "summary":
		return notifier.Send(FormatScheduledSummary(artefacts, summaryForReleases))

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

	default:
		if resolver != nil {
			res := resolver.Resolve(ctx, sessionID, msg)
			switch res.Kind {
			case intent.Dispatched:
				// Re-dispatch with the resolved command; no further intent resolution.
				// logFetcher/llm/launchpad are nil — investigate cannot be invoked via intent resolver.
				return Dispatch(ctx, sessionID, res.Command, artefacts, defaultRelease, summaryForProducts, summaryForReleases, notifier, "", nil, nil, nil, nil)
			case intent.NeedsInfo:
				return notifier.Send(res.Reply)
			case intent.Failed:
				return notifier.Send(res.Reply)
			}
		}
		return notifier.Send(fmt.Sprintf("I didn't understand `%s`. Type `help` for available commands.", msg))
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
