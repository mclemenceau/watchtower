package intent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mclemenceau/watchtower/internal/llm"
)

// jsonResp builds a minimal intent JSON response string.
func jsonResp(command string, confidence float64, clarification string) string {
	return `{"command":"` + command + `","confidence":` + floatStr(confidence) + `,"clarification":"` + clarification + `"}`
}

func floatStr(f float64) string {
	if f == 0.9 {
		return "0.9"
	}
	if f == 0.3 {
		return "0.3"
	}
	if f == 0.2 {
		return "0.2"
	}
	return "0.0"
}

func TestResolve_Dispatched(t *testing.T) {
	mock := &llm.MockLLMClient{
		Response: jsonResp("builds status noble", 0.9, ""),
	}
	r := New(mock)

	res := r.Resolve(context.Background(), "sess1", "show me failing noble builds")

	if res.Kind != Dispatched {
		t.Fatalf("expected Dispatched, got %v", res.Kind)
	}
	if res.Command != "builds status noble" {
		t.Errorf("expected command 'builds status noble', got %q", res.Command)
	}
}

func TestResolve_NeedsInfo(t *testing.T) {
	mock := &llm.MockLLMClient{
		Response: jsonResp("", 0.2, "Do you want build status or test status?"),
	}
	r := New(mock)

	res := r.Resolve(context.Background(), "sess2", "status")

	if res.Kind != NeedsInfo {
		t.Fatalf("expected NeedsInfo, got %v", res.Kind)
	}
	if !strings.Contains(res.Reply, "build status or test status") {
		t.Errorf("unexpected clarification reply: %q", res.Reply)
	}
}

func TestResolve_Failed_LLMError(t *testing.T) {
	mock := &llm.MockLLMClient{
		Err: errors.New("network timeout"),
	}
	r := New(mock)

	res := r.Resolve(context.Background(), "sess3", "anything")

	if res.Kind != Failed {
		t.Fatalf("expected Failed, got %v", res.Kind)
	}
	if !strings.Contains(res.Reply, "network timeout") {
		t.Errorf("expected error in reply, got %q", res.Reply)
	}
}

func TestResolve_Failed_BadJSON(t *testing.T) {
	mock := &llm.MockLLMClient{
		Response: "not json at all",
	}
	r := New(mock)

	res := r.Resolve(context.Background(), "sess4", "anything")

	if res.Kind != Failed {
		t.Fatalf("expected Failed, got %v", res.Kind)
	}
}

func TestResolve_MultiTurn(t *testing.T) {
	// First call: ambiguous → clarification
	// Second call (answer): confident → dispatch
	responses := []string{
		jsonResp("", 0.2, "Do you want build status or test status?"),
		jsonResp("builds status", 0.9, ""),
	}
	i := 0
	mock := &llm.MockLLMClient{}

	r := New(mock)

	// Override Complete to return responses in sequence.
	seq := &seqClient{responses: responses}
	r.llm = seq

	// Turn 1: ambiguous
	res1 := r.Resolve(context.Background(), "sess5", "status")
	if res1.Kind != NeedsInfo {
		t.Fatalf("turn 1: expected NeedsInfo, got %v", res1.Kind)
	}

	// Verify session was stored.
	r.mu.Lock()
	_, hasSess := r.sessions["sess5"]
	r.mu.Unlock()
	if !hasSess {
		t.Fatal("expected session to be stored after NeedsInfo")
	}

	// Turn 2: answer
	res2 := r.Resolve(context.Background(), "sess5", "builds")
	if res2.Kind != Dispatched {
		t.Fatalf("turn 2: expected Dispatched, got %v (reply: %q)", res2.Kind, res2.Reply)
	}
	if res2.Command != "builds status" {
		t.Errorf("expected 'builds status', got %q", res2.Command)
	}

	// Session should be cleaned up after dispatch.
	r.mu.Lock()
	_, stillHasSess := r.sessions["sess5"]
	r.mu.Unlock()
	if stillHasSess {
		t.Fatal("expected session to be cleared after Dispatched")
	}

	_ = i
}

func TestResolve_SessionClearedAfterFailure(t *testing.T) {
	// Seed a pending session manually, then simulate an LLM error.
	mock := &llm.MockLLMClient{Err: errors.New("timeout")}
	r := New(mock)

	r.mu.Lock()
	r.sessions["sess6"] = &session{history: []string{"status", "builds or tests?"}}
	r.mu.Unlock()

	res := r.Resolve(context.Background(), "sess6", "builds")
	if res.Kind != Failed {
		t.Fatalf("expected Failed, got %v", res.Kind)
	}

	r.mu.Lock()
	_, still := r.sessions["sess6"]
	r.mu.Unlock()
	if still {
		t.Fatal("expected session cleared after LLM failure")
	}
}

func TestParseIntentResponse_StripsFences(t *testing.T) {
	raw := "```json\n{\"command\":\"builds status\",\"confidence\":0.9,\"clarification\":\"\"}\n```"
	resp, err := parseIntentResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Command != "builds status" {
		t.Errorf("expected 'builds status', got %q", resp.Command)
	}
}

// seqClient returns responses in sequence, cycling when exhausted.
type seqClient struct {
	responses []string
	idx       int
}

func (s *seqClient) Complete(_ context.Context, _, _ string) (string, error) {
	if len(s.responses) == 0 {
		return "", errors.New("no responses configured")
	}
	resp := s.responses[s.idx%len(s.responses)]
	s.idx++
	return resp, nil
}
