package intent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mclemenceau/watchtower/internal/adapters/openrouter"
)

// jsonResp builds a minimal intent JSON response string (no answer field).
func jsonResp(command string, confidence float64, clarification string) string {
	return `{"command":"` + command + `","confidence":` + floatStr(confidence) + `,"clarification":"` + clarification + `","answer":""}`
}

// jsonRespAnswered builds an intent JSON response string with a direct answer.
func jsonRespAnswered(answer string) string {
	return `{"command":"","confidence":0.0,"clarification":"","answer":"` + answer + `"}`
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
	mock := &openrouter.MockLLMClient{
		Response: jsonResp("builds status noble", 0.9, ""),
	}
	r := New(mock)

	res := r.Resolve(context.Background(), "sess1", "show me failing noble builds", "")

	if res.Kind != Dispatched {
		t.Fatalf("expected Dispatched, got %v", res.Kind)
	}
	if res.Command != "builds status noble" {
		t.Errorf("expected command 'builds status noble', got %q", res.Command)
	}
}

func TestResolve_NeedsInfo(t *testing.T) {
	mock := &openrouter.MockLLMClient{
		Response: jsonResp("", 0.2, "Do you want build status or test status?"),
	}
	r := New(mock)

	res := r.Resolve(context.Background(), "sess2", "status", "")

	if res.Kind != NeedsInfo {
		t.Fatalf("expected NeedsInfo, got %v", res.Kind)
	}
	if !strings.Contains(res.Reply, "build status or test status") {
		t.Errorf("unexpected clarification reply: %q", res.Reply)
	}
}

func TestResolve_Answered(t *testing.T) {
	mock := &openrouter.MockLLMClient{
		Response: jsonRespAnswered("**2 noble builds failed**: noble-desktop-amd64 (ID 1), noble-desktop-arm64 (ID 2)."),
	}
	r := New(mock)

	res := r.Resolve(context.Background(), "sess-ans", "how many noble builds failed?", `{"artefacts":[],"failures":[]}`)

	if res.Kind != Answered {
		t.Fatalf("expected Answered, got %v", res.Kind)
	}
	if !strings.Contains(res.Reply, "noble builds failed") {
		t.Errorf("unexpected answer reply: %q", res.Reply)
	}
	// Session should be cleared after a direct answer.
	r.mu.Lock()
	_, still := r.sessions["sess-ans"]
	r.mu.Unlock()
	if still {
		t.Fatal("expected session cleared after Answered")
	}
}

func TestResolve_Answered_WithContext(t *testing.T) {
	// Verify that contextJSON is forwarded to the LLM prompt (seqClient captures the prompt).
	captured := &capturingClient{response: jsonRespAnswered("3 failures.")}
	r := New(captured)

	ctxJSON := `{"artefacts":[{"id":1,"name":"noble-desktop-amd64","release":"noble"}],"failures":[]}`
	_ = r.Resolve(context.Background(), "sess-ctx", "any failures?", ctxJSON)

	if !strings.Contains(captured.lastPrompt, ctxJSON) {
		t.Errorf("expected contextJSON in prompt, got: %q", captured.lastPrompt)
	}
}

func TestResolve_Failed_LLMError(t *testing.T) {
	mock := &openrouter.MockLLMClient{
		Err: errors.New("network timeout"),
	}
	r := New(mock)

	res := r.Resolve(context.Background(), "sess3", "anything", "")

	if res.Kind != Failed {
		t.Fatalf("expected Failed, got %v", res.Kind)
	}
	if !strings.Contains(res.Reply, "network timeout") {
		t.Errorf("expected error in reply, got %q", res.Reply)
	}
}

func TestResolve_Failed_BadJSON(t *testing.T) {
	mock := &openrouter.MockLLMClient{
		Response: "not json at all",
	}
	r := New(mock)

	res := r.Resolve(context.Background(), "sess4", "anything", "")

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
	mock := &openrouter.MockLLMClient{}

	r := New(mock)

	// Override Complete to return responses in sequence.
	seq := &seqClient{responses: responses}
	r.llm = seq

	// Turn 1: ambiguous
	res1 := r.Resolve(context.Background(), "sess5", "status", "")
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
	res2 := r.Resolve(context.Background(), "sess5", "builds", "")
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
}

func TestResolve_MultiTurn_ContextReinjected(t *testing.T) {
	// Verify that contextJSON is included in the multi-turn prompt (turn 2).
	responses := []string{
		jsonResp("", 0.2, "Which release?"),
		jsonRespAnswered("noble has 1 failure."),
	}
	capturing := &capturingClient{responses: responses}
	r := New(capturing)

	ctxJSON := `{"artefacts":[],"failures":[]}`

	// Turn 1: ambiguous clarification.
	r.Resolve(context.Background(), "sess-mt", "any failures?", ctxJSON)
	// Turn 2: contextJSON must reappear in the multi-turn prompt.
	r.Resolve(context.Background(), "sess-mt", "noble", ctxJSON)

	if !strings.Contains(capturing.lastPrompt, ctxJSON) {
		t.Errorf("expected contextJSON re-injected on turn 2, got: %q", capturing.lastPrompt)
	}
}

func TestResolve_SessionClearedAfterFailure(t *testing.T) {
	// Seed a pending session manually, then simulate an LLM error.
	mock := &openrouter.MockLLMClient{Err: errors.New("timeout")}
	r := New(mock)

	r.mu.Lock()
	r.sessions["sess6"] = &session{history: []string{"status", "builds or tests?"}}
	r.mu.Unlock()

	res := r.Resolve(context.Background(), "sess6", "builds", "")
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
	raw := "```json\n{\"command\":\"builds status\",\"confidence\":0.9,\"clarification\":\"\",\"answer\":\"\"}\n```"
	resp, err := parseIntentResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Command != "builds status" {
		t.Errorf("expected 'builds status', got %q", resp.Command)
	}
}

func TestParseIntentResponse_AnswerField(t *testing.T) {
	raw := `{"command":"","confidence":0.0,"clarification":"","answer":"3 failures found."}`
	resp, err := parseIntentResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Answer != "3 failures found." {
		t.Errorf("expected answer '3 failures found.', got %q", resp.Answer)
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

// capturingClient records the last prompt it received and returns configured responses.
type capturingClient struct {
	lastPrompt string
	response   string   // used when responses is empty
	responses  []string // used in sequence when set
	idx        int
}

func (c *capturingClient) Complete(_ context.Context, _, prompt string) (string, error) {
	c.lastPrompt = prompt
	if len(c.responses) > 0 {
		resp := c.responses[c.idx%len(c.responses)]
		c.idx++
		return resp, nil
	}
	return c.response, nil
}
