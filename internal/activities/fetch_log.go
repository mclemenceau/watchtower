package activities

import (
	"context"
	"fmt"
	"strings"
)

// FetchLog retrieves the log at logURL via the injected LogFetcher and returns
// the last 200 lines.
func (a *Activities) FetchLog(ctx context.Context, logURL string) (string, error) {
	content, err := a.LogFetcher.Fetch(ctx, logURL)
	if err != nil {
		return "", fmt.Errorf("FetchLog: %w", err)
	}
	return lastN(content, 200), nil
}

func lastN(text string, n int) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= n {
		return text
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}
