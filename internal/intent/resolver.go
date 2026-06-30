// Package intent provides LLM-assisted resolution of free-text user messages
// into structured bot commands.
//
// When a message does not match any known keyword pattern, the Resolver asks an
// LLM to either map it to a known command (high confidence) or produce a focused
// clarifying question (low confidence / ambiguous). Multi-turn clarification is
// supported via a lightweight in-memory session map.
package intent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mclemenceau/watchtower/internal/llm"
)

// sessionTTL is how long a pending clarification session lives without activity.
const sessionTTL = 5 * time.Minute

// systemPrompt describes the bot's capabilities to the LLM and instructs it on
// the response format. It is static and computed once at init time.
const systemPrompt = `You are Watchtower, a Ubuntu image build pipeline monitoring bot.
Your job is to map a user's free-text message to one of the supported commands listed below,
or ask a single focused clarifying question when the message is ambiguous.

Supported commands:
  builds status
  builds status <release>
  builds status <release> <product>
  tests status
  tests status <release>
  tests status <release> <product>
  help

Rules:
1. If you can confidently map the message to a command, respond with JSON:
   {"command":"<command>","confidence":0.9,"clarification":""}
2. If the message is ambiguous or missing a required argument (e.g. release or product),
   respond with JSON:
   {"command":"","confidence":0.3,"clarification":"<one short focused question>"}
3. Never ask for information you already have from the message.
4. Never invent commands outside the list above.
5. Always respond with valid JSON only — no prose, no markdown fences.

Examples:
  User: "show me failing noble builds"
  → {"command":"builds status noble","confidence":0.9,"clarification":""}

  User: "status"
  → {"command":"","confidence":0.2,"clarification":"Do you want build status or test status?"}

  User: "build status" (after clarification answer "noble")
  → {"command":"builds status noble","confidence":0.9,"clarification":""}
`

// intentResponse is the JSON structure the LLM is instructed to return.
type intentResponse struct {
	Command       string  `json:"command"`
	Confidence    float64 `json:"confidence"`
	Clarification string  `json:"clarification"`
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
)

// Resolution is the result of a single Resolve call.
type Resolution struct {
	Kind    ResolutionKind
	Command string // set when Kind == Dispatched
	Reply   string // set when Kind == NeedsInfo or Failed
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
	llm      llm.LLMClient
	mu       sync.Mutex
	sessions map[string]*session
}

// New creates a Resolver backed by the given LLMClient.
func New(client llm.LLMClient) *Resolver {
	return &Resolver{
		llm:      client,
		sessions: make(map[string]*session),
	}
}

// confidenceThreshold is the minimum confidence score to act without asking.
const confidenceThreshold = 0.7

// Resolve interprets msg for the given sessionID (e.g. "repl", channelID+userID).
//
//   - If a pending clarification session exists for sessionID, the message is
//     treated as the answer to the outstanding question and the conversation
//     continues.
//   - Otherwise a fresh LLM call is made.
//
// The caller should pass the result to mattermost.Dispatch when Kind==Dispatched,
// send Resolution.Reply to the user when Kind==NeedsInfo or Kind==Failed.
func (r *Resolver) Resolve(ctx context.Context, sessionID, msg string) Resolution {
	r.mu.Lock()
	r.evictExpired()
	sess := r.sessions[sessionID]
	r.mu.Unlock()

	var prompt string
	if sess != nil {
		// Continue multi-turn: build a compact context block.
		var sb strings.Builder
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
		prompt = msg
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
