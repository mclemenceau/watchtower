package testapi

import "github.com/mclemenceau/watchtower/internal/buildapi"

// IsDisplayable returns true when the execution represents a real test result
// worth surfacing to the user. Filtering rules:
//   - "Image build" is always skipped — it is a build availability notification,
//     not a test result.
//   - "Manual Testing" with status IN_PROGRESS is skipped — it means no tester
//     has submitted results yet (placeholder execution with no test_results).
func IsDisplayable(te buildapi.TestExecution) bool {
	if te.TestPlan == "Image build" {
		return false
	}
	if te.TestPlan == "Manual Testing" && te.Status == "IN_PROGRESS" {
		return false
	}
	return true
}

// ExecStatusEmoji returns an emoji for a TestExecution status string.
func ExecStatusEmoji(status string) string {
	switch status {
	case "PASSED":
		return "✅"
	case "FAILED":
		return "❌"
	case "IN_PROGRESS":
		return "🔄"
	case "NOT_STARTED":
		return "⏳"
	default:
		return "⚠️"
	}
}
