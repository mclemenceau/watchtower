package activities

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mclemenceau/watchtower/internal/application"
	"github.com/mclemenceau/watchtower/internal/domain"
	"github.com/mclemenceau/watchtower/internal/logutil"
	"github.com/mclemenceau/watchtower/internal/ports"
	"github.com/mclemenceau/watchtower/internal/state"
)

// Activities holds the dependencies injected at worker startup.
type Activities struct {
	Artefacts          ports.ArtefactSource
	Tests              ports.BuildSource
	Snapshot           ports.SnapshotStore
	Failures           ports.FailureStorePort
	Hook               ports.Notifier
	LogFetcher         ports.LogFetcher
	Launchpad          ports.LaunchpadSource // optional; enables two-hop LP log resolution
	ReleasesScope      []string              // ordered release scope for all operations; nil = all
	SummaryForProducts []string              // restrict summaries to these OS/product names; nil = all
	LLM                ports.LLMClient
	MaxAnalysisPerRun  int // cap on LLM calls per FailureAnalysisWorkflow run; 0 = default (5)
}

func (a *Activities) FetchBuildStatus(ctx context.Context) ([]domain.Artefact, error) {
	artefacts, err := a.Artefacts.FetchArtefacts(ctx)
	if err != nil {
		return nil, fmt.Errorf("FetchBuildStatus: %w", err)
	}
	// Apply release scope early: discard artefacts outside the configured scope
	// so that all downstream operations (FetchTestExecutions, Diff, storage) only
	// handle the releases we care about, reducing API call count and storage size.
	if len(a.ReleasesScope) > 0 {
		var scoped []domain.Artefact
		for _, art := range artefacts {
			if releaseInScope(a.ReleasesScope, art.Release) {
				scoped = append(scoped, art)
			}
		}
		artefacts = scoped
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

// EnrichBuildStatus populates the BuildLog field on each artefact by fetching
// today's cd-build-log and parsing the per-arch Launchpad status lines.
// For artefacts already built today (version date == today) the status is set
// to BuildStatusBuilt without fetching any log.
// For others the log is fetched; individual fetch failures are non-fatal:
//   - HTTP 404 (today)  → fall back to yesterday's log before giving up
//   - HTTP 404 (both)   → BuildStatusNotStarted (log not yet published)
//   - Other error       → BuildStatusUnknown
//   - Fetch succeeded   → ParseBuildStatusFromLog using artefact arch
//
// The yesterday fallback handles the common case where today's build has not
// started yet but yesterday's log contains a meaningful result (e.g. LP built
// successfully but cdimage crashed before submitting to Test Observer).
//
// The enriched slice is returned; the input slice is not modified.
func (a *Activities) EnrichBuildStatus(ctx context.Context, artefacts []domain.Artefact) ([]domain.Artefact, error) {
	enriched := make([]domain.Artefact, len(artefacts))
	copy(enriched, artefacts)

	if a.LogFetcher == nil {
		// No fetcher wired — leave BuildLog unset; formatters fall back to version-based status.
		return enriched, nil
	}

	now := time.Now().UTC()
	today := now.Format("20060102")
	yesterday := now.AddDate(0, 0, -1).Format("20060102")

	for i, art := range enriched {
		// Artefacts with today's serial are already confirmed built.
		if domain.IsBuiltToday(art.Version) {
			enriched[i].BuildLog = domain.BuildStatusBuilt
			continue
		}

		logURL := domain.LogURLFromImageURLForDate(art.ImageURL, today)
		if logURL == "" {
			// No log URL derivable (missing or unrecognised ImageURL) — unknown.
			enriched[i].BuildLog = domain.BuildStatusUnknown
			continue
		}

		arch := domain.ArtefactArch(art.Name)
		logPrefix := domain.LogPrefixFromImageURL(art.ImageURL)
		label := domain.ResolveLogLabel(logPrefix, arch)

		content, err := a.LogFetcher.Fetch(ctx, logURL)
		if err != nil {
			if !errors.Is(err, domain.ErrLogNotFound) {
				enriched[i].BuildLog = domain.BuildStatusUnknown
				continue
			}
			// Today's log is not published yet — fall back to yesterday's log.
			// This surfaces meaningful results (e.g. LP succeeded but cdimage
			// crashed before submitting to Test Observer) rather than reporting
			// NOT_STARTED when there is actually useful status information.
			yesterdayURL := domain.LogURLFromImageURLForDate(art.ImageURL, yesterday)
			content, err = a.LogFetcher.Fetch(ctx, yesterdayURL)
			if err != nil {
				if errors.Is(err, domain.ErrLogNotFound) {
					enriched[i].BuildLog = domain.BuildStatusNotStarted
				} else {
					enriched[i].BuildLog = domain.BuildStatusUnknown
				}
				continue
			}
		}

		// Resolve the canonical log label for this (logPrefix, arch) pair.
		// For most artefacts logPrefix is e.g. "daily-live" and the label equals
		// the arch. For preinstalled builds the log uses "{arch}-{variant}" labels;
		// ResolveLogLabel returns the appropriate canonical variant (e.g. "amd64-generic").
		status, failureKind, failureDesc := domain.ParseBuildStatusFromLog(content, label)
		enriched[i].BuildLog = status
		enriched[i].BuildFailureKind = failureKind
		enriched[i].BuildFailureDescription = failureDesc
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
// If ReleasesScope has entries it uses the first as the pinned release;
// otherwise it falls back to auto-detecting the most active release.
func (a *Activities) FormatStatusTable(_ context.Context, artefacts []domain.Artefact) (string, error) {
	release := ""
	if len(a.ReleasesScope) > 0 {
		release = a.ReleasesScope[0]
	}
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
			art.Name, art.OS, art.Release, domain.ImageAge(art.Version), buildStatusCell(art), domain.LogCell(art.ImageURL))
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

	msg := application.FormatScheduledSummary(artefacts, a.ReleasesScope)
	return a.NotifyChannel(ctx, msg)
}

// NotifyNewBuilds posts a compact notification listing every artefact whose build
// completed successfully since the previous snapshot. It is a no-op when the report
// contains no new builds, so callers do not need to guard the call.
func (a *Activities) NotifyNewBuilds(ctx context.Context, report domain.ChangeReport) error {
	if len(report.NewBuilds) == 0 {
		return nil
	}
	msg := application.FormatNewBuildsNotification(report.NewBuilds)
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

// releaseInScope reports whether release (case-insensitive) is in the scope list.
func releaseInScope(scope []string, release string) bool {
	for _, r := range scope {
		if strings.EqualFold(r, release) {
			return true
		}
	}
	return false
}

// LoadFailures reads the current FailureStore from disk.
func (a *Activities) LoadFailures(_ context.Context) (domain.FailureStore, error) {
	if a.Failures == nil {
		return make(domain.FailureStore), nil
	}
	store, err := a.Failures.ReadFailures()
	if err != nil {
		return nil, fmt.Errorf("LoadFailures: %w", err)
	}
	return store, nil
}

// SaveFailures persists the FailureStore to disk.
func (a *Activities) SaveFailures(_ context.Context, store domain.FailureStore) error {
	if a.Failures == nil {
		return nil
	}
	if err := a.Failures.WriteFailures(store); err != nil {
		return fmt.Errorf("SaveFailures: %w", err)
	}
	return nil
}

// UpdateFailureRecords merges a ChangeReport into the persisted FailureStore:
//   - NewFailures  → upsert (create or increment occurrences)
//   - NewArtefacts → upsert when already MARKED_AS_FAILED (first-boot seeding)
//   - Recoveries   → mark resolved (silently, no notification per spec)
//
// The artefacts slice is used to look up Release and OS for each delta, since
// ArtefactDelta only carries Name/Release/Version, not OS/product.
// NewArtefacts handling is critical for the first-boot case: when Watchtower
// starts with an empty failures.json but a snapshot that already contains
// MARKED_AS_FAILED artefacts, Diff produces NewArtefacts (not NewFailures)
// because there is no previous status to transition from. Without this path
// those failures would never be recorded.
func (a *Activities) UpdateFailureRecords(_ context.Context, report domain.ChangeReport, artefacts []domain.Artefact) error {
	if a.Failures == nil {
		return nil
	}

	store, err := a.Failures.ReadFailures()
	if err != nil {
		return fmt.Errorf("UpdateFailureRecords: read: %w", err)
	}

	// Build a quick name→artefact index (name is unique enough for this purpose).
	byName := make(map[string]domain.Artefact, len(artefacts))
	for _, art := range artefacts {
		byName[art.Name] = art
	}

	for _, delta := range report.NewFailures {
		art, ok := byName[delta.Name]
		if !ok {
			continue
		}
		store.UpsertFailure(art)
	}

	// Seed failures for brand-new artefacts that are already MARKED_AS_FAILED.
	// This covers the first-boot case where no old snapshot exists so Diff
	// cannot observe a status transition.
	for _, art := range report.NewArtefacts {
		if art.Status == "MARKED_AS_FAILED" {
			store.UpsertFailure(art)
		}
	}

	for _, delta := range report.Recoveries {
		art, ok := byName[delta.Name]
		if !ok {
			continue
		}
		store.ResolveFailure(art.ID, art.Release, art.OS)
	}

	if err := a.Failures.WriteFailures(store); err != nil {
		return fmt.Errorf("UpdateFailureRecords: write: %w", err)
	}
	return nil
}

// AnalyseFailures runs LLM log analysis on unresolved FailureRecords that have
// no Analysis yet. It processes at most maxPerRun records (token cap). Results
// are persisted back to failures.json after each successful analysis.
//
// When a.Launchpad is configured the two-hop log resolution is used: it first
// fetches the cd-build-log, then resolves the per-arch Launchpad librarian URL
// and fetches the actual builder log. This gives the LLM the detailed error
// output (e.g. debootstrap failures, dependency conflicts) rather than just a
// high-level "Failed to build" line. Falls back to the cd-build-log if any
// step in the resolution fails.
func (a *Activities) AnalyseFailures(ctx context.Context, artefacts []domain.Artefact) error {
	if a.Failures == nil || a.LLM == nil || a.LogFetcher == nil {
		return nil // LLM or fetcher not configured — skip silently
	}

	store, err := a.Failures.ReadFailures()
	if err != nil {
		return fmt.Errorf("AnalyseFailures: read: %w", err)
	}

	maxPerRun := a.MaxAnalysisPerRun
	if maxPerRun <= 0 {
		maxPerRun = 5
	}

	pending := store.PendingAnalysis(maxPerRun)
	if len(pending) == 0 {
		return nil
	}

	// Build artefact index by ID for log URL resolution.
	byID := make(map[int]domain.Artefact, len(artefacts))
	for _, art := range artefacts {
		byID[art.ID] = art
	}

	for _, rec := range pending {
		art, ok := byID[rec.ArtefactID]
		if !ok {
			continue
		}

		// Use the two-hop resolution: cd-build-log → LP REST → librarian.
		// a.Launchpad may be nil; ResolveLogURL gracefully falls back to the
		// cd-build-log in that case, so INFRA failures still get analysed.
		analysis, _, err := logutil.AnalyzeLog(
			ctx, art, a.LogFetcher, a.Launchpad, a.LLM,
		)
		if err != nil {
			// Non-fatal: one bad LLM or fetch call should not abort the run.
			continue
		}
		store.SetAnalysis(
			rec.ArtefactID, rec.Release, rec.Product,
			analysis, rec.LastSeenVersion,
		)
	}

	if err := a.Failures.WriteFailures(store); err != nil {
		return fmt.Errorf("AnalyseFailures: write: %w", err)
	}
	return nil
}

// buildStatusCell returns the status emoji/icon for an artefact in the status table.
// When a BuildLog state has been populated (via EnrichBuildStatus), it takes precedence
// and shows the richer 5-state icon. Otherwise it falls back to the simple version-based
// binary status (IsBuiltToday → ✅ / ❌).
func buildStatusCell(art domain.Artefact) string {
	if art.BuildLog != "" {
		return domain.BuildLogIcon(art.BuildLog)
	}
	return domain.BuildStatus(art.Version)
}
