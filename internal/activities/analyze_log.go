package activities

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mclemenceau/watchtower/internal/domain"
	"github.com/mclemenceau/watchtower/internal/logutil"
)

// AnalyzeLog is a Temporal activity wrapper that runs LLM root-cause analysis
// on pre-fetched log content. The imageID is used only as context in the prompt.
//
// For the full two-hop log resolution + analysis path used by AnalyseFailures,
// see logutil.AnalyzeLog.
func (a *Activities) AnalyzeLog(ctx context.Context, imageID, logContent string) (domain.LogAnalysis, error) {
	truncated := logutil.LastNLines(logContent, 200)
	prompt := fmt.Sprintf(
		"Image: %s\n\nBuild log (last 200 lines):\n%s",
		imageID, truncated,
	)

	raw, err := a.LLM.Complete(ctx, logutil.AnalyzeLogSystem, prompt)
	if err != nil {
		return domain.LogAnalysis{}, fmt.Errorf("AnalyzeLog: %w", err)
	}

	var result domain.LogAnalysis
	if err := json.Unmarshal([]byte(logutil.StripCodeFence(raw)), &result); err != nil {
		return domain.LogAnalysis{}, fmt.Errorf("AnalyzeLog: parse response: %w", err)
	}
	return result, nil
}
