package activities

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mclemenceau/watchtower/internal/domain"
	"github.com/mclemenceau/watchtower/internal/llm"
	"github.com/mclemenceau/watchtower/internal/mattermost"
	"github.com/mclemenceau/watchtower/internal/state"
)

// Activities holds the dependencies injected at worker startup.
type Activities struct {
	Artefacts      artefactClient
	Tests          testClient
	Snapshot       *state.Snapshot
	Hook           mattermost.WebhookClient
	DefaultRelease string // pin status table to this release; empty = auto-detect
	// TODO: wire LLM when log analysis is implemented
	LLM llm.LLMClient
}

// artefactClient is the minimal interface needed from buildapi.ArtefactClient.
type artefactClient interface {
	FetchArtefacts(ctx context.Context) ([]domain.Artefact, error)
}

// testClient is the minimal interface needed from testapi.TestClient.
type testClient interface {
	FetchBuilds(ctx context.Context, artefactID int) ([]domain.ArtefactBuild, error)
}

func (a *Activities) FetchBuildStatus(ctx context.Context) ([]domain.Artefact, error) {
	artefacts, err := a.Artefacts.FetchArtefacts(ctx)
	if err != nil {
		return nil, fmt.Errorf("FetchBuildStatus: %w", err)
	}
	return artefacts, nil
}

// FetchTestExecutions enriches each artefact with its build/test execution data
// by calling the Test Observer API once per artefact. Errors for individual
// artefacts are logged and skipped rather than aborting the whole fetch.
func (a *Activities) FetchTestExecutions(ctx context.Context, artefacts []domain.Artefact) ([]domain.Artefact, error) {
	enriched := make([]domain.Artefact, len(artefacts))
	copy(enriched, artefacts)
	for i, art := range enriched {
		builds, err := a.Tests.FetchBuilds(ctx, art.ID)
		if err != nil {
			// Non-fatal: leave Builds empty for this artefact.
			continue
		}
		enriched[i].Builds = builds
	}
	return enriched, nil
}

func (a *Activities) LoadSnapshot(_ context.Context) ([]domain.Artefact, error) {
	artefacts, err := a.Snapshot.Read()
	if err != nil {
		return nil, fmt.Errorf("LoadSnapshot: %w", err)
	}
	return artefacts, nil
}

func (a *Activities) SaveSnapshot(_ context.Context, artefacts []domain.Artefact) error {
	if err := a.Snapshot.Write(artefacts); err != nil {
		return fmt.Errorf("SaveSnapshot: %w", err)
	}
	return nil
}

// FormatStatusTable renders a status table for the configured release.
// If DefaultRelease is empty it falls back to auto-detecting the most active release.
func (a *Activities) FormatStatusTable(_ context.Context, artefacts []domain.Artefact) (string, error) {
	release := a.DefaultRelease
	if release == "" {
		release = state.LatestRelease(artefacts)
	}

	var filtered []domain.Artefact
	for _, art := range artefacts {
		if art.Release == release {
			filtered = append(filtered, art)
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].OS != filtered[j].OS {
			return filtered[i].OS < filtered[j].OS
		}
		return filtered[i].Name < filtered[j].Name
	})

	var sb strings.Builder
	fmt.Fprintf(&sb, "**Build Status — %s** · %s\n\n",
		release, time.Now().UTC().Format("2006-01-02 15:04 UTC"))
	sb.WriteString("| Name | Product | Release | Age | Status | Log |\n")
	sb.WriteString("|------|---------|---------|-----|--------|-----|\n")
	for _, art := range filtered {
		fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s | %s |\n",
			art.Name, art.OS, art.Release, domain.ImageAge(art.Version), domain.BuildStatus(art.Version), domain.LogCell(art.ImageURL))
	}
	return sb.String(), nil
}

// NotifyChannel sends a message to the Mattermost channel (or stdout in simulation mode).
func (a *Activities) NotifyChannel(_ context.Context, text string) error {
	if err := a.Hook.Send(text); err != nil {
		return fmt.Errorf("NotifyChannel: %w", err)
	}
	return nil
}
