package mattermost_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	mattermostadapter "github.com/mclemenceau/watchtower/internal/adapters/mattermost"
	"github.com/mclemenceau/watchtower/internal/state"
)

// TestChannelNotifierSend verifies that ChannelNotifier POSTs to /api/v4/posts
// with the correct Authorization header and message body.
func TestChannelNotifierSend(t *testing.T) {
	var (
		mu           sync.Mutex
		receivedAuth string
		receivedMsg  string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/posts" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var req map[string]string
		_ = json.NewDecoder(r.Body).Decode(&req)

		mu.Lock()
		receivedAuth = r.Header.Get("Authorization")
		receivedMsg = req["message"]
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"post1"}`))
	}))
	defer srv.Close()

	n := mattermostadapter.NewChannelNotifier(srv.URL, "mytoken", "ch123")
	if err := n.Send("hello world"); err != nil {
		t.Fatalf("Send returned unexpected error: %v", err)
	}

	mu.Lock()
	gotAuth, gotMsg := receivedAuth, receivedMsg
	mu.Unlock()

	if gotAuth != "Bearer mytoken" {
		t.Errorf("expected Authorization header %q, got %q", "Bearer mytoken", gotAuth)
	}
	if gotMsg != "hello world" {
		t.Errorf("expected message %q, got %q", "hello world", gotMsg)
	}
}

// TestRunBotDisabledWhenNoCreds verifies that RunBot is a no-op when credentials are missing.
func TestRunBotDisabledWhenNoCreds(t *testing.T) {
	var httpCalled atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpCalled.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	snap := state.New(t.TempDir() + "/snap.json")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// No token — RunBot must return immediately without making any network call.
	mattermostadapter.RunBot(ctx, mattermostadapter.BotConfig{
		ServerURL: srv.URL,
		BotUserID: "bot1",
		Keyword:   "@watchtower",
	}, snap, "", nil, nil, nil, nil, nil, nil, nil)

	if httpCalled.Load() {
		t.Error("RunBot should not make any HTTP/WS calls when Token is missing")
	}
}

// wsUpgrader is a shared WebSocket upgrader used in tests.
var wsUpgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

// postedEvent builds a Mattermost "posted" WebSocket event for a user post.
// id is the post ID, rootID is non-empty when the post is a thread reply.
func postedEvent(channelID, userID, id, rootID, message string) map[string]interface{} {
	postJSON, _ := json.Marshal(map[string]interface{}{
		"id":         id,
		"channel_id": channelID,
		"user_id":    userID,
		"message":    message,
		"type":       "",
		"root_id":    rootID,
	})
	return map[string]interface{}{
		"event": "posted",
		"data": map[string]interface{}{
			"post": string(postJSON),
		},
		"broadcast": map[string]interface{}{
			"channel_id": channelID,
			"user_id":    userID,
		},
	}
}

// botPostedEvent simulates the bot's own post arriving on the WebSocket.
// The bot ignores it for dispatch but registers its ID in botThreads.
func botPostedEvent(channelID, botUserID, id, rootID string) map[string]interface{} {
	postJSON, _ := json.Marshal(map[string]interface{}{
		"id":         id,
		"channel_id": channelID,
		"user_id":    botUserID,
		"message":    "some bot reply",
		"type":       "",
		"root_id":    rootID,
	})
	return map[string]interface{}{
		"event": "posted",
		"data": map[string]interface{}{
			"post": string(postJSON),
		},
		"broadcast": map[string]interface{}{
			"channel_id": channelID,
			"user_id":    botUserID,
		},
	}
}

// newTestServer creates a combined HTTP+WebSocket test server.
// wsEvents is a function that is called once the WS connection is established
// (after consuming the auth challenge) to deliver events to the bot.
// Each POST to /api/v4/posts calls onPost with the request body map.
func newTestServer(t *testing.T, wsEvents func(conn *websocket.Conn), onPost func(map[string]string)) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v4/websocket", func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()           //nolint:errcheck
		_, _, _ = conn.ReadMessage() // consume auth challenge
		wsEvents(conn)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})

	mux.HandleFunc("/api/v4/posts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var req map[string]string
			_ = json.NewDecoder(r.Body).Decode(&req)
			if onPost != nil {
				onPost(req)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"bot-reply-1"}`))
	})

	return httptest.NewServer(mux)
}

// TestRunBotDispatchesKeywordMention verifies that RunBot dispatches a command
// and posts a response when a @watchtower mention arrives over WebSocket.
func TestRunBotDispatchesKeywordMention(t *testing.T) {
	var (
		mu        sync.Mutex
		postedMsg string
	)

	srv := newTestServer(t,
		func(conn *websocket.Conn) {
			_ = conn.WriteJSON(postedEvent("ch1", "user42", "post-user-1", "", "@watchtower help"))
		},
		func(req map[string]string) {
			mu.Lock()
			postedMsg = req["message"]
			mu.Unlock()
		},
	)
	defer srv.Close()

	snap := state.New(t.TempDir() + "/snap.json")
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	mattermostadapter.RunBot(ctx, mattermostadapter.BotConfig{
		ServerURL:      srv.URL,
		Token:          "testtoken",
		BotUserID:      "botuser",
		Keyword:        "@watchtower",
		ReconnectDelay: 10 * time.Millisecond,
	}, snap, "", nil, nil, nil, nil, nil, nil, nil)

	mu.Lock()
	got := postedMsg
	mu.Unlock()

	if !strings.Contains(got, "builds status") {
		t.Errorf("expected help text in posted reply, got: %q", got)
	}
}

// TestRunBotRespondsToThreadReplyWithoutKeyword verifies that the bot replies
// to a thread follow-up that contains no keyword, as long as the thread was
// started by the bot itself.
func TestRunBotRespondsToThreadReplyWithoutKeyword(t *testing.T) {
	var (
		mu         sync.Mutex
		postedMsgs []string
	)

	srv := newTestServer(t,
		func(conn *websocket.Conn) {
			// Step 1: user sends an @watchtower command.
			_ = conn.WriteJSON(postedEvent("ch1", "user42", "post-user-1", "", "@watchtower summary"))

			// Give the bot a moment to process and (in the test server) record the reply.
			time.Sleep(50 * time.Millisecond)

			// Step 2: simulate the bot's own reply arriving on the WebSocket so
			// the session registers "bot-reply-1" as a known bot post.
			_ = conn.WriteJSON(botPostedEvent("ch1", "botuser", "bot-reply-1", "post-user-1"))

			// Step 3: user replies in the thread with no keyword at all.
			_ = conn.WriteJSON(postedEvent("ch1", "user42", "post-user-2", "post-user-1", "can you clarify the noble status?"))
		},
		func(req map[string]string) {
			mu.Lock()
			postedMsgs = append(postedMsgs, req["message"])
			mu.Unlock()
		},
	)
	defer srv.Close()

	snap := state.New(t.TempDir() + "/snap.json")
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()

	mattermostadapter.RunBot(ctx, mattermostadapter.BotConfig{
		ServerURL:      srv.URL,
		Token:          "testtoken",
		BotUserID:      "botuser",
		Keyword:        "@watchtower",
		ReconnectDelay: 10 * time.Millisecond,
	}, snap, "", nil, nil, nil, nil, nil, nil, nil)

	mu.Lock()
	count := len(postedMsgs)
	mu.Unlock()

	// Expect at least two replies: one for the initial command, one for the
	// keyword-free thread follow-up.
	if count < 2 {
		t.Fatalf("expected at least 2 bot posts (initial reply + thread follow-up), got %d: %v", count, postedMsgs)
	}
}
