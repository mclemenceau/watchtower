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
		_, _ = w.Write([]byte(`{}`))
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

// postedEvent builds a Mattermost "posted" WebSocket event payload.
func postedEvent(channelID, userID, message string) map[string]interface{} {
	postJSON, _ := json.Marshal(map[string]interface{}{
		"id":         "post1",
		"channel_id": channelID,
		"user_id":    userID,
		"message":    message,
		"type":       "",
		"root_id":    "",
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

// TestRunBotDispatchesKeywordMention verifies that RunBot dispatches a command
// and posts a response when a @watchtower mention arrives over WebSocket.
func TestRunBotDispatchesKeywordMention(t *testing.T) {
	var (
		mu        sync.Mutex
		postedMsg string
	)

	// Single test server: handles both /api/v4/websocket and /api/v4/posts.
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v4/websocket", func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close() //nolint:errcheck

		// Consume the authentication challenge the bot sends on connect.
		_, _, _ = conn.ReadMessage()

		// Deliver a single @watchtower help mention.
		_ = conn.WriteJSON(postedEvent("ch1", "user42", "@watchtower help"))

		// Hold the connection open until the client closes it.
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
			mu.Lock()
			postedMsg = req["message"]
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	})

	srv := httptest.NewServer(mux)
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

	// After the context expires the bot must have dispatched "help" and posted a reply.
	mu.Lock()
	got := postedMsg
	mu.Unlock()

	if !strings.Contains(got, "builds status") {
		t.Errorf("expected help text in posted reply, got: %q", got)
	}
}
