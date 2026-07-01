// Package application provides the protocol-agnostic command routing layer.
// It imports only domain and ports — never any adapter package.
package application

import (
	"context"
	"fmt"
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
func Dispatch(ctx context.Context, sessionID, msg string, artefacts []domain.Artefact, defaultRelease string, notifier ports.Notifier, keyword string, resolver *intent.Resolver) error {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return nil
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

	default:
		if resolver != nil {
			res := resolver.Resolve(ctx, sessionID, msg)
			switch res.Kind {
			case intent.Dispatched:
				// Re-dispatch with the resolved command; no further intent resolution.
				return Dispatch(ctx, sessionID, res.Command, artefacts, defaultRelease, notifier, "", nil)
			case intent.NeedsInfo:
				return notifier.Send(res.Reply)
			case intent.Failed:
				return notifier.Send(res.Reply)
			}
		}
		return notifier.Send(fmt.Sprintf("I didn't understand `%s`. Type `help` for available commands.", msg))
	}
}
