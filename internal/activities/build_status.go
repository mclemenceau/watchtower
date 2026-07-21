package activities

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mclemenceau/watchtower/internal/application"
	"github.com/mclemenceau/watchtower/internal/domain"
	"github.com/mclemenceau/watchtower/internal/ports"
	"github.com/mclemenceau/watchtower/internal/state"
)

// Activities holds the dependencies injected at worker startup.
type Activities struct {
	Artefacts          ports.ArtefactSource
	Tests              ports.BuildSource
	Snapshot           ports.SnapshotStore
	Hook               ports.Notifier
	LogFetcher         ports.LogFetcher
	DefaultRelease     string   // pin status table to this release; empty = auto-detect
	SummaryForReleases []string // ordered release list for scheduled summary; nil = all
	SummaryForProducts []string // restrict summaries to these OS/product names; nil = all
	LLM                ports.LLMClient
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
		if art.Release != release {
			continue
		}
		if len(a.SummaryForProducts) > 0 && !containsSummaryProduct(a.SummaryForProducts, art.OS) {
			continue
		}
		filtered = append(filtered, art)
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

// NotifyChannel sends a message to the notification channel.
func (a *Activities) NotifyChannel(_ context.Context, text string) error {
	if err := a.Hook.Send(text); err != nil {
		return fmt.Errorf("NotifyChannel: %w", err)
	}
	return nil
}

// PostSummary reads the current snapshot, applies the SummaryForReleases and
// SummaryForProducts filters, formats the scheduled build summary, and posts it
// to the notification channel.
func (a *Activities) PostSummary(ctx context.Context) error {
	artefacts, err := a.Snapshot.Read()
	if err != nil {
		return fmt.Errorf("PostSummary: read snapshot: %w", err)
	}

	// Apply product filter (same logic as Dispatch).
	if len(a.SummaryForProducts) > 0 {
		var filtered []domain.Artefact
		for _, art := range artefacts {
			if containsSummaryProduct(a.SummaryForProducts, art.OS) {
				filtered = append(filtered, art)
			}
		}
		artefacts = filtered
	}

	msg := application.FormatScheduledSummary(artefacts, a.SummaryForReleases)
	return a.NotifyChannel(ctx, msg)
}

// containsSummaryProduct reports whether product (case-insensitive) is present in the list.
func containsSummaryProduct(list []string, product string) bool {
	for _, p := range list {
		if strings.EqualFold(p, product) {
			return true
		}
	}
	return false
}
