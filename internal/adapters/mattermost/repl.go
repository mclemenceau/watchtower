package mattermost

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mclemenceau/watchtower/internal/application"
	"github.com/mclemenceau/watchtower/internal/intent"
	"github.com/mclemenceau/watchtower/internal/ports"
	"github.com/mclemenceau/watchtower/internal/state"
)

// RunREPL reads lines from in (simulating incoming Mattermost messages), dispatches
// each to application.Dispatch, and sends replies via notifier. Blocks until in is
// closed or ctx is cancelled. defaultRelease pins the status table to a specific
// release (empty = auto-detect from snapshot). summaryForProducts restricts all views
// to those product/OS names (nil = all). summaryForReleases is the ordered release
// list used by the `summary` command (nil = all). keyword is the optional trigger
// prefix (e.g. "@watchtower"); pass empty string to dispatch every line regardless
// of prefix. resolver is optional; pass nil to disable LLM-assisted intent resolution.
// logFetcher, llm, and launchpad are optional; pass nil to disable the investigate command.
func RunREPL(ctx context.Context, in io.Reader, notifier ports.Notifier, snap *state.Snapshot, defaultRelease string, summaryForProducts []string, summaryForReleases []string, keyword string, resolver *intent.Resolver, logFetcher ports.LogFetcher, llm ports.LLMClient, launchpad ports.LaunchpadSource) {
	fmt.Println("[Watchtower] Bot started. Type a message (Ctrl-D to quit):")
	fmt.Print("you> ")

	scanner := bufio.NewScanner(in)
	for {
		// Check context before blocking on next line.
		select {
		case <-ctx.Done():
			fmt.Println("\n[Watchtower] Shutting down.")
			return
		default:
		}

		if !scanner.Scan() {
			fmt.Println("\n[Watchtower] Shutting down.")
			return
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			fmt.Print("you> ")
			continue
		}

		artefacts, err := snap.Read()
		if err != nil {
			fmt.Printf("[Watchtower] error reading snapshot: %v\n", err)
			fmt.Print("you> ")
			continue
		}

		if dispatchErr := application.Dispatch(ctx, "repl", line, artefacts, defaultRelease, summaryForProducts, summaryForReleases, notifier, keyword, resolver, logFetcher, llm, launchpad); dispatchErr != nil {
			fmt.Printf("[Watchtower] error: %v\n", dispatchErr)
		}

		fmt.Print("you> ")
	}
}
