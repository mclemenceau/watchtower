package application

import (
	"context"

	"github.com/mclemenceau/watchtower/internal/domain"
	"github.com/mclemenceau/watchtower/internal/logutil"
	"github.com/mclemenceau/watchtower/internal/ports"
)

// analyzeLog fetches the best available log for the artefact and runs LLM
// root-cause analysis on it. Returns the analysis and the human-readable
// source description for display.
//
// It delegates all log resolution and LLM call logic to logutil so the same
// implementation is shared with the background FailureAnalysisWorkflow.
func analyzeLog(
	ctx context.Context,
	art domain.Artefact,
	logFetcher ports.LogFetcher,
	launchpad ports.LaunchpadSource,
	llm ports.LLMClient,
) (domain.LogAnalysis, string, error) {
	analysis, src, err := logutil.AnalyzeLog(ctx, art, logFetcher, launchpad, llm)
	if err != nil {
		return domain.LogAnalysis{}, "", err
	}
	return analysis, src.Description, nil
}
