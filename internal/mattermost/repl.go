package mattermost

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mclemenceau/argus/internal/buildapi"
	"github.com/mclemenceau/argus/internal/state"
)

// SnapshotReader is a function that returns the current artefact snapshot.
// Used by the REPL to pass fresh data to Dispatch on every message.
type SnapshotReader func() ([]buildapi.Artefact, error)

// RunREPL reads lines from in (simulating incoming Mattermost messages), dispatches
// each to Dispatch, and sends replies via hook. Blocks until in is closed or ctx
// is cancelled. defaultRelease pins the status table to a specific release (empty =
// auto-detect from snapshot). keyword is the optional trigger prefix (e.g.
// "@watchtower"); pass empty string to dispatch every line regardless of prefix.
func RunREPL(ctx context.Context, in io.Reader, hook WebhookClient, snap *state.Snapshot, defaultRelease string, keyword string) {
	fmt.Println("[ARGUS] Bot started. Type a message (Ctrl-D to quit):")
	fmt.Print("you> ")

	scanner := bufio.NewScanner(in)
	for {
		// Check context before blocking on next line.
		select {
		case <-ctx.Done():
			fmt.Println("\n[ARGUS] Shutting down.")
			return
		default:
		}

		if !scanner.Scan() {
			fmt.Println("\n[ARGUS] Shutting down.")
			return
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			fmt.Print("you> ")
			continue
		}

		artefacts, err := snap.Read()
		if err != nil {
			fmt.Printf("[ARGUS] error reading snapshot: %v\n", err)
			fmt.Print("you> ")
			continue
		}

		if dispatchErr := Dispatch(line, artefacts, defaultRelease, hook, keyword); dispatchErr != nil {
			fmt.Printf("[ARGUS] error: %v\n", dispatchErr)
		}

		fmt.Print("you> ")
	}
}
