package activities

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mclemenceau/argus/internal/buildapi"
)

const answerQuerySystem = `You are Argus, a helpful Ubuntu release engineer assistant monitoring image build pipelines.
You are given a snapshot of all current Ubuntu image artefacts and a user question.
Answer the question directly and helpfully using only the provided data.

Response format:
- Single artefact question: 2-4 sentences of prose. Mention name, product, release, version, age, and status.
- List or overview question: one brief summary sentence, then a Markdown table with columns:
  | Name | Product | Release | Age | Status |
  Put failures first unless the user specifies otherwise.
- Comparison or cross-cutting question: use your best judgment, tables are fine.

Status meanings: APPROVED = QA signed-off ✅, MARKED_AS_FAILED = flagged by reviewers ❌, UNDECIDED = awaiting review ⏳.
Use those emoji in tables. You may use Markdown (bold, italic, inline code, tables). No HTML.
The snapshot is refreshed automatically every ~10 minutes; mention this only if freshness is directly relevant.`

// AnswerQuery sends the full artefact snapshot and the user's query to the LLM in a single call.
func (a *Activities) AnswerQuery(ctx context.Context, query string, artefacts []buildapi.Artefact) (buildapi.AgentReply, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "User question: %q\n\nArtefact snapshot (%d artefacts, as of %s):\n\n",
		query, len(artefacts), time.Now().UTC().Format("15:04 UTC"))

	sb.WriteString("| Name | Product | Release | Version | Age | Status |\n")
	sb.WriteString("|------|---------|---------|---------|-----|--------|\n")
	for _, art := range artefacts {
		fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s | %s |\n",
			art.Name, art.OS, art.Release, art.Version, imageAge(art.Version), art.Status)
	}

	answer, err := a.LLM.Complete(ctx, answerQuerySystem, sb.String())
	if err != nil {
		return buildapi.AgentReply{}, fmt.Errorf("AnswerQuery: %w", err)
	}
	return buildapi.AgentReply{Summary: answer}, nil
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
	case days <= 0:
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
