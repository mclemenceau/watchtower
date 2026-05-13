package activities

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mclemenceau/argus/internal/buildapi"
)

const composeReplySystem = `You are a helpful Ubuntu release engineer assistant.
Write a concise, human-friendly response (2-4 sentences) summarising the artefact situation.
Be direct and specific — mention the image name, release, version (build date), and current status.
Status values: APPROVED means QA-signed-off, MARKED_AS_FAILED means reviewers flagged a problem,
UNDECIDED means awaiting human review.
You may use Markdown: **bold** and inline code. No headers or bullet points.`

const composeListSystem = `You are a helpful Ubuntu release engineer assistant.
Given a list of Ubuntu image artefacts and the user's query, respond with:
1. One brief sentence summarising the overview (e.g. how many artefacts, any failures).
2. A Markdown table with columns: | Name | Product | Release | Age | Status |
Sort and filter the table according to the user's intent — put failures first unless specified otherwise.
Status rendering: APPROVED → ✅ approved, MARKED_AS_FAILED → ❌ failed, UNDECIDED → ⏳ pending.
Age is already computed and provided as a human-readable string — use it as-is.
Use only Markdown. No HTML.`

// ComposeReply asks the LLM to write a human-readable summary and packages it into an AgentReply.
// Pass a non-nil analysis for failure diagnosis flows; nil for simple status queries.
func (a *Activities) ComposeReply(ctx context.Context, artefact buildapi.Artefact, analysis *LogAnalysis) (buildapi.AgentReply, error) {
	prompt := buildComposePrompt(artefact, analysis)

	summary, err := a.LLM.Complete(ctx, composeReplySystem, prompt)
	if err != nil {
		return buildapi.AgentReply{}, fmt.Errorf("ComposeReply: %w", err)
	}

	reply := buildapi.AgentReply{Summary: summary}
	if analysis != nil {
		reply.Category    = analysis.Category
		reply.Hypothesis  = analysis.Hypothesis
		reply.LogExcerpts = analysis.LogExcerpts
		reply.NextAction  = analysis.NextAction
	} else {
		reply.Category = categoryFromStatus(artefact.Status)
	}
	return reply, nil
}

// ComposeListReply asks the LLM to produce a markdown table for a list of artefacts.
func (a *Activities) ComposeListReply(ctx context.Context, query string, artefacts []buildapi.Artefact) (buildapi.AgentReply, error) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("User query: %q\n\nArtefacts (%d):\n", query, len(artefacts)))
	sb.WriteString("Name | Product | Release | Age | Status\n")
	for _, art := range artefacts {
		sb.WriteString(fmt.Sprintf("%s | %s | %s | %s | %s\n",
			art.Name, art.OS, art.Release, imageAge(art.Version), art.Status))
	}

	summary, err := a.LLM.Complete(ctx, composeListSystem, sb.String())
	if err != nil {
		return buildapi.AgentReply{}, fmt.Errorf("ComposeListReply: %w", err)
	}
	return buildapi.AgentReply{Summary: summary}, nil
}

func buildComposePrompt(artefact buildapi.Artefact, analysis *LogAnalysis) string {
	base := fmt.Sprintf(
		"Artefact: %s\nProduct: %s\nRelease: %s\nVersion: %s\nAge: %s\nStatus: %s",
		artefact.Name, artefact.OS, artefact.Release,
		artefact.Version, imageAge(artefact.Version), artefact.Status,
	)

	if artefact.ImageURL != "" {
		base += fmt.Sprintf("\nImage URL: %s", artefact.ImageURL)
	}

	if analysis != nil {
		base += fmt.Sprintf("\n\nRoot cause analysis:\nCategory: %s\nHypothesis: %s\nNext action: %s",
			analysis.Category, analysis.Hypothesis, analysis.NextAction)
	}

	return base
}

// imageAge returns a human-readable age string for a YYYYMMDD or YYYYMMDD.N version field.
func imageAge(version string) string {
	// Strip respin suffix (e.g. "20240513.2" → "20240513")
	if i := strings.IndexByte(version, '.'); i != -1 {
		version = version[:i]
	}
	if len(version) != 8 {
		return "unknown"
	}
	t, err := time.Parse("20060102", version)
	if err != nil {
		return "unknown"
	}
	days := int(time.Since(t).Hours() / 24)
	switch {
	case days < 0:
		return "today"
	case days == 0:
		return "today"
	case days == 1:
		return "1 day"
	case days < 14:
		return fmt.Sprintf("%d days", days)
	case days < 60:
		weeks := days / 7
		if weeks == 1 {
			return "1 week"
		}
		return fmt.Sprintf("%d weeks", weeks)
	default:
		months := days / 30
		if months == 1 {
			return "1 month"
		}
		return fmt.Sprintf("%d months", months)
	}
}

func categoryFromStatus(status string) string {
	switch status {
	case "APPROVED":
		return "approved"
	case "MARKED_AS_FAILED":
		return "failed"
	case "UNDECIDED":
		return "pending"
	default:
		return ""
	}
}
