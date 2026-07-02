package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mclemenceau/watchtower/internal/domain"
)

const analyzeLogSystem = `You are a build failure analyst for Ubuntu image builds.
Given a build log, identify the root cause of the failure.
Respond with valid JSON only — no markdown, no extra text:
{
  "category": "infra|code|dependency|flaky|unknown",
  "hypothesis": "one-sentence root cause",
  "log_excerpts": ["most relevant line 1", "most relevant line 2"],
  "next_action": "recommended next step for the engineer"
}`

func (a *Activities) AnalyzeLog(ctx context.Context, imageID, logContent string) (domain.LogAnalysis, error) {
	prompt := fmt.Sprintf("Image: %s\n\nBuild log (last 200 lines):\n%s", imageID, logContent)

	raw, err := a.LLM.Complete(ctx, analyzeLogSystem, prompt)
	if err != nil {
		return domain.LogAnalysis{}, fmt.Errorf("AnalyzeLog: %w", err)
	}

	var result domain.LogAnalysis
	if err := json.Unmarshal([]byte(stripCodeFence(raw)), &result); err != nil {
		return domain.LogAnalysis{}, fmt.Errorf("AnalyzeLog: parse response: %w", err)
	}
	return result, nil
}

// stripCodeFence removes ```json ... ``` wrappers that some models add.
func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	if i := strings.Index(s, "\n"); i != -1 {
		s = s[i+1:]
	}
	if i := strings.LastIndex(s, "```"); i != -1 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
