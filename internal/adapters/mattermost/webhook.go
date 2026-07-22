// Package mattermost provides the Mattermost adapter implementing ports.Notifier.
package mattermost

import (
	"fmt"

	"github.com/mclemenceau/watchtower/internal/ports"
)

// Compile-time interface satisfaction check.
var _ ports.Notifier = (*StdoutNotifier)(nil)

// StdoutNotifier writes messages to stdout — simulates a Mattermost channel in dev/REPL mode.
// Each message is prefixed with "[Watchtower →]" to distinguish agent output from user input.
type StdoutNotifier struct{}

// Send prints the message to stdout.
func (s *StdoutNotifier) Send(text string) error {
	fmt.Printf("\n[Watchtower →]\n%s\n", text)
	return nil
}
