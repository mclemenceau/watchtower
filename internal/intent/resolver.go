// Package intent provides LLM-assisted resolution of free-text user messages
// into structured bot commands.
//
// When a message does not match any known keyword pattern, the Resolver asks an
// LLM to either:
//   - map it to a known command (high confidence) → ResolutionKind Dispatched
//   - answer directly using the provided pipeline state → ResolutionKind Answered
//   - produce a focused clarifying question (ambiguous) → ResolutionKind NeedsInfo
//
// Multi-turn clarification is supported via a lightweight in-memory session map.
// State context (contextJSON) is re-injected on every turn so answers always
// reflect the latest snapshot.
package intent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mclemenceau/watchtower/internal/ports"
)

// sessionTTL is how long a pending clarification session lives without activity.
const sessionTTL = 5 * time.Minute

// systemPrompt describes the bot's capabilities to the LLM and instructs it on
// the response format. It is static and computed once at init time.
const systemPrompt = `You are Watchtower, a Ubuntu image build pipeline monitoring bot.

You receive a user question and, when available, a JSON snapshot of the current
pipeline state under the key "state". Use it to answer data questions directly.

Supported commands:
  builds status
  builds status <release>
  builds status <release> <product>
  tests status
  tests status <release>
  tests status <release> <product>
  failures
  failures <release>
  failures <release> <product>
  analyse failures
  analyse failures <release>
  investigate <artefact-id>
  help

Respond with JSON in exactly one of these three forms:

1. COMMAND — you can map the question to a known command with high confidence:
   {"command":"builds status noble","confidence":0.9,"clarification":"","answer":""}

2. ANSWER — the question requires reasoning over the state data provided; answer
   in concise markdown prose, citing artefact names/IDs where relevant:
   {"command":"","confidence":0.0,"clarification":"","answer":"**3 noble builds failed today**: noble-desktop-amd64 (ID 1234), ..."}

3. CLARIFY — the question is ambiguous and you cannot answer or map it without
   more information; ask one focused question. For investigate requests with
   multiple matching artefacts, list them with IDs:
   {"command":"","confidence":0.3,"clarification":"Which artefact? I found:\n- 1234 noble-desktop-amd64\n- 1235 noble-desktop-arm64","answer":""}

Rules:
1. Prefer COMMAND when the question maps cleanly to a supported command.
2. Use ANSWER only when interpretation or reasoning over state data is needed.
3. Never invent data not present in the provided state snapshot.
4. Never ask for information you already have from the message.
5. Never invent commands outside the list above.
6. Always respond with valid JSON only — no prose, no markdown fences.

Examples:
  User: "show me failing noble builds"
  → {"command":"builds status noble","confidence":0.9,"clarification":"","answer":""}

  User: "how many noble desktop builds failed this week?"
  → {"command":"","confidence":0.0,"clarification":"","answer":"**2 noble desktop builds failed**: noble-desktop-amd64 (ID 1234) first seen 20240715, noble-desktop-arm64 (ID 1235) first seen 20240716."}

  User: "what's wrong with noble desktop?"
  → {"command":"","confidence":0.3,"clarification":"I found 2 noble desktop artefacts. Which one do you want to investigate?\n- 1234 noble-desktop-amd64\n- 1235 noble-desktop-arm64","answer":""}

  User: "what failures do we have?"
  → {"command":"failures","confidence":0.9,"clarification":"","answer":""}

  User: "status"
  → {"command":"","confidence":0.2,"clarification":"Do you want build status, test status, or failure status?","answer":""}
`

// intentResponse is the JSON structure the LLM is instructed to return.
type intentResponse struct {
	Command       string  `json:"command"`
	Confidence    float64 `json:"confidence"`
	Clarification string  `json:"clarification"`
	Answer        string  `json:"answer"`
}

// ResolutionKind describes the outcome of a Resolve call.
type ResolutionKind int

const (
	// Dispatched means the LLM mapped the message to a known command.
	// Resolution.Command holds the command string ready to pass to Dispatch.
	Dispatched ResolutionKind = iota
	// NeedsInfo means the LLM could not resolve confidently and has posed a
	// clarifying question. Resolution.Reply holds the question text.
	NeedsInfo
	// Failed means the LLM call itself failed. Resolution.Reply holds an
	// error message suitable for the user.
	Failed
	// Answered means the LLM answered the question directly using the provided
	// state context. Resolution.Reply holds the prose markdown answer.
	Answered
)

// Resolution is the result of a single Resolve call.
type Resolution struct {
	Kind    ResolutionKind
	Command string // set when Kind == Dispatched
	Reply   string // set when Kind == NeedsInfo, Failed, or Answered
}

// session holds a pending clarification conversation for one user/channel pair.
type session struct {
	// history accumulates the multi-turn context sent back to the LLM.
	// Format: alternating user/assistant messages as plain text.
	history   []string
	createdAt time.Time
}

// Resolver maps free-text messages to bot commands using an LLM.
// It is safe for concurrent use.
type Resolver struct {
	llm      ports.LLMClient
	mu       sync.Mutex
	sessions map[string]*session
}

// New creates a Resolver backed by the given LLMClient.
func New(client ports.LLMClient) *Resolver {
	return &Resolver{
		llm:      client,
		sessions: make(map[string]*session),
	}
}

// confidenceThreshold is the minimum confidence score to act without asking.
const confidenceThreshold = 0.7

// Resolve interprets msg for the given sessionID (e.g. "repl", channelID+userID).
//
// contextJSON is a pre-serialised JSON snapshot of the relevant pipeline state
// (artefacts and failures). It is injected into the LLM prompt on every call so
// that answers always reflect the latest data. Pass an empty string when no state
// is available (e.g. on first boot before any snapshot has been fetched).
//
//   - If a pending clarification session exists for sessionID, the message is
//     treated as the answer to the outstanding question and the conversation
//     continues. contextJSON is re-injected fresh on every turn.
//   - Otherwise a fresh LLM call is made.
//
// The caller should:
//   - Pass the resolved command to application.Dispatch when Kind == Dispatched.
//   - Send Resolution.Reply to the user when Kind == NeedsInfo, Answered, or Failed.
func (r *Resolver) Resolve(ctx context.Context, sessionID, msg, contextJSON string) Resolution {
	r.mu.Lock()
	r.evictExpired()
	sess := r.sessions[sessionID]
	r.mu.Unlock()

	var prompt string
	if sess != nil {
		// Continue multi-turn: build a compact context block including fresh state.
		var sb strings.Builder
		if contextJSON != "" {
			fmt.Fprintf(&sb, "state: %s\n\n", contextJSON)
		}
		for i, line := range sess.history {
			if i%2 == 0 {
				fmt.Fprintf(&sb, "User: %s\n", line)
			} else {
				fmt.Fprintf(&sb, "Assistant: %s\n", line)
			}
		}
		fmt.Fprintf(&sb, "User: %s", msg)
		prompt = sb.String()
	} else {
		if contextJSON != "" {
			prompt = fmt.Sprintf("state: %s\n\n%s", contextJSON, msg)
		} else {
			prompt = msg
		}
	}

	raw, err := r.llm.Complete(ctx, systemPrompt, prompt)
	if err != nil {
		r.mu.Lock()
		delete(r.sessions, sessionID)
		r.mu.Unlock()
		return Resolution{
			Kind:  Failed,
			Reply: fmt.Sprintf("I couldn't process your request right now (%s). Try a specific command or type `help`.", err.Error()),
		}
	}

	resp, err := parseIntentResponse(raw)
	if err != nil {
		r.mu.Lock()
		delete(r.sessions, sessionID)
		r.mu.Unlock()
		return Resolution{
			Kind:  Failed,
			Reply: "I couldn't process your request right now (unexpected LLM response). Try a specific command or type `help`.",
		}
	}

	// Direct answer: LLM responded with prose using the state data.
	if resp.Answer != "" {
		r.mu.Lock()
		delete(r.sessions, sessionID)
		r.mu.Unlock()
		return Resolution{
			Kind:  Answered,
			Reply: resp.Answer,
		}
	}

	if resp.Confidence >= confidenceThreshold && resp.Command != "" {
		// Confident match — clear any pending session and dispatch.
		r.mu.Lock()
		delete(r.sessions, sessionID)
		r.mu.Unlock()
		return Resolution{
			Kind:    Dispatched,
			Command: resp.Command,
		}
	}

	// Ambiguous — store/extend the session and ask for clarification.
	r.mu.Lock()
	if sess == nil {
		sess = &session{createdAt: time.Now()}
		r.sessions[sessionID] = sess
	}
	sess.history = append(sess.history, msg, resp.Clarification)
	r.mu.Unlock()

	question := resp.Clarification
	if question == "" {
		question = "Could you be more specific? Type `help` to see available commands."
	}
	return Resolution{
		Kind:  NeedsInfo,
		Reply: question,
	}
}

// parseIntentResponse extracts the JSON intent payload from the LLM output.
// It strips optional markdown code fences before unmarshalling.
func parseIntentResponse(raw string) (intentResponse, error) {
	s := strings.TrimSpace(raw)
	// Strip ```json ... ``` fences some models add.
	if strings.HasPrefix(s, "```") {
		if i := strings.Index(s, "\n"); i != -1 {
			s = s[i+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
		s = strings.TrimSpace(s)
	}
	var resp intentResponse
	if err := json.Unmarshal([]byte(s), &resp); err != nil {
		return intentResponse{}, fmt.Errorf("parseIntentResponse: %w", err)
	}
	return resp, nil
}

// evictExpired removes sessions that have exceeded sessionTTL.
// Must be called with r.mu held.
func (r *Resolver) evictExpired() {
	cutoff := time.Now().Add(-sessionTTL)
	for id, s := range r.sessions {
		if s.createdAt.Before(cutoff) {
			delete(r.sessions, id)
		}
	}
}
