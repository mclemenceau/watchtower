package activities

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mclemenceau/argus/internal/buildapi"
	"github.com/mclemenceau/argus/internal/llm"
	"github.com/mclemenceau/argus/internal/state"
)

// Activities holds the dependencies injected at worker startup.
type Activities struct {
	Artefacts      buildapi.ArtefactClient
	Snapshot       *state.Snapshot
	LLM            llm.LLMClient
	FeedURL        string // base URL of the HTTP server for SSE push
	DefaultRelease string // pin status table to this release; empty = auto-detect
}

func (a *Activities) FetchBuildStatus(ctx context.Context) ([]buildapi.Artefact, error) {
	artefacts, err := a.Artefacts.FetchArtefacts(ctx)
	if err != nil {
		return nil, fmt.Errorf("FetchBuildStatus: %w", err)
	}
	return artefacts, nil
}

func (a *Activities) LoadSnapshot(_ context.Context) ([]buildapi.Artefact, error) {
	artefacts, err := a.Snapshot.Read()
	if err != nil {
		return nil, fmt.Errorf("LoadSnapshot: %w", err)
	}
	return artefacts, nil
}

func (a *Activities) SaveSnapshot(_ context.Context, artefacts []buildapi.Artefact) error {
	if err := a.Snapshot.Write(artefacts); err != nil {
		return fmt.Errorf("SaveSnapshot: %w", err)
	}
	return nil
}

// FormatStatusTable renders a status table for the configured release.
// If DefaultRelease is empty it falls back to auto-detecting the most active release.
func (a *Activities) FormatStatusTable(_ context.Context, artefacts []buildapi.Artefact) (string, error) {
	release := a.DefaultRelease
	if release == "" {
		release = state.LatestRelease(artefacts)
	}

	var filtered []buildapi.Artefact
	for _, art := range artefacts {
		if art.Release == release {
			filtered = append(filtered, art)
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "**Build Status — %s** · %s\n\n",
		release, time.Now().UTC().Format("2006-01-02 15:04 UTC"))
	sb.WriteString("| Name | Product | Release | Age | Status |\n")
	sb.WriteString("|------|---------|---------|-----|--------|\n")
	for _, art := range filtered {
		fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s |\n",
			art.Name, art.OS, art.Release, imageAge(art.Version), statusEmoji(art.Status))
	}
	return sb.String(), nil
}

func statusEmoji(status string) string {
	switch status {
	case "APPROVED":
		return "✅ approved"
	case "MARKED_AS_FAILED":
		return "❌ failed"
	default:
		return "⏳ pending"
	}
}
