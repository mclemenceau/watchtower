package mattermost_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mattermostadapter "github.com/mclemenceau/watchtower/internal/adapters/mattermost"
	"github.com/mclemenceau/watchtower/internal/state"
)

// captureNotifier records the last message sent via Send.
type captureNotifier struct {
	last string
}

func (c *captureNotifier) Send(text string) error {
	c.last = text
	return nil
}

// mmPostList and mmPost mirror the unexported types in the adapter.
type mmPostList struct {
	Order []string          `json:"order"`
	Posts map[string]mmPost `json:"posts"`
}

type mmPost struct {
	ID       string `json:"id"`
	Message  string `json:"message"`
	CreateAt int64  `json:"create_at"`
	UserId   string `json:"user_id"`
}

// TestRunPollerDispatchesKeywordPosts verifies that RunPoller only dispatches
// posts containing the keyword and ignores others.
func TestRunPollerDispatchesKeywordPosts(t *testing.T) {
	now := time.Now().UnixMilli()
	posts := map[string]mmPost{
		"p1": {ID: "p1", Message: "@watchtower help", CreateAt: now + 1000},
		"p2": {ID: "p2", Message: "random noise", CreateAt: now + 2000},
	}
	pl := mmPostList{
		Order: []string{"p2", "p1"},
		Posts: posts,
	}
	body, _ := json.Marshal(pl)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	hook := &captureNotifier{}

	// Use a temp snapshot (no file on disk — returns nil artefacts).
	snap := state.New(t.TempDir() + "/snap.json")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	cfg := mattermostadapter.PollerConfig{
		ServerURL: srv.URL,
		Token:     "tok",
		ChannelID: "ch",
		Interval:  50 * time.Millisecond,
		Keyword:   "@watchtower",
	}

	mattermostadapter.RunPoller(ctx, cfg, snap, "", nil, nil, hook, srv.Client(), nil, nil, nil, nil)

	// After the context expires the poller should have dispatched the "help" command.
	if !strings.Contains(hook.last, "builds status") {
		t.Errorf("expected help output after dispatching '@watchtower help', got: %s", hook.last)
	}
}

// TestRunPollerDisabledWhenNoCreds verifies that RunPoller is a no-op when
// essential credentials are missing.
func TestRunPollerDisabledWhenNoCreds(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	hook := &captureNotifier{}
	snap := state.New(t.TempDir() + "/snap.json")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// No token — poller must return immediately without making any HTTP call.
	mattermostadapter.RunPoller(ctx, mattermostadapter.PollerConfig{
		ServerURL: srv.URL,
		ChannelID: "ch",
		Interval:  10 * time.Millisecond,
		Keyword:   "@watchtower",
	}, snap, "", nil, nil, hook, srv.Client(), nil, nil, nil, nil)

	if called {
		t.Error("poller should not make HTTP calls when Token is missing")
	}
}
