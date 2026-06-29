package mattermost

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mclemenceau/argus/internal/buildapi"
	"github.com/mclemenceau/argus/internal/state"
)

// --- Dispatch keyword filtering ---

func TestDispatchKeywordRequired(t *testing.T) {
	hook := &captureHook{}
	// With keyword set, a bare "help" (no keyword prefix) must be ignored.
	if err := Dispatch("help", testArtefacts, "", hook, "@watchtower"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hook.last != "" {
		t.Errorf("message without keyword should be ignored, got: %s", hook.last)
	}
}

func TestDispatchKeywordStripped(t *testing.T) {
	hook := &captureHook{}
	// "@watchtower help" must route to the help handler.
	if err := Dispatch("@watchtower help", testArtefacts, "", hook, "@watchtower"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "builds status") {
		t.Errorf("keyword-prefixed help should produce help output, got: %s", hook.last)
	}
}

func TestDispatchKeywordCaseInsensitive(t *testing.T) {
	hook := &captureHook{}
	if err := Dispatch("@Watchtower builds status", testArtefacts, "", hook, "@watchtower"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "noble") {
		t.Errorf("keyword match should be case-insensitive, got: %s", hook.last)
	}
}

func TestDispatchKeywordBareShowsHelp(t *testing.T) {
	hook := &captureHook{}
	// Just the keyword alone (no command) should show help.
	if err := Dispatch("@watchtower", testArtefacts, "", hook, "@watchtower"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "builds status") {
		t.Errorf("bare keyword should show help, got: %s", hook.last)
	}
}

func TestDispatchNoKeyword(t *testing.T) {
	hook := &captureHook{}
	// Empty keyword → every message is routed without filtering.
	if err := Dispatch("help", testArtefacts, "", hook, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "builds status") {
		t.Errorf("empty keyword: help should still produce output, got: %s", hook.last)
	}
}

// --- fetchNewPosts / RunPoller integration via httptest ---

func TestFetchNewPostsFiltersAndCursorAdvances(t *testing.T) {
	now := time.Now().UnixMilli()

	// Build a fake Mattermost POST list response.
	posts := map[string]mmPost{
		"post1": {ID: "post1", Message: "@watchtower help", CreateAt: now + 1000, UserId: "u1"},
		"post2": {ID: "post2", Message: "just chatting", CreateAt: now + 2000, UserId: "u2"},
	}
	pl := mmPostList{
		Order: []string{"post2", "post1"}, // newest first (Mattermost convention)
		Posts: posts,
	}
	body, _ := json.Marshal(pl)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer mytoken" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	result, newSince, err := fetchNewPosts(context.Background(), srv.Client(), PollerConfig{
		ServerURL: srv.URL,
		Token:     "mytoken",
		ChannelID: "mychannel",
	}, now)
	if err != nil {
		t.Fatalf("fetchNewPosts error: %v", err)
	}
	// Both posts returned, ordered oldest-first.
	if len(result) != 2 {
		t.Fatalf("expected 2 posts, got %d", len(result))
	}
	if result[0].ID != "post1" || result[1].ID != "post2" {
		t.Errorf("posts not in chronological order: %v %v", result[0].ID, result[1].ID)
	}
	// Cursor must have advanced past the newest post.
	if newSince <= now+2000 {
		t.Errorf("cursor should advance beyond newest post (%d), got %d", now+2000, newSince)
	}
}

func TestFetchNewPostsUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, _, err := fetchNewPosts(context.Background(), srv.Client(), PollerConfig{
		ServerURL: srv.URL,
		Token:     "badtoken",
		ChannelID: "ch",
	}, 0)
	if err == nil {
		t.Fatal("expected error for non-200 response, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention 401, got: %v", err)
	}
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

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	hook := &captureHook{}

	// Use a temp snapshot (no file on disk — returns nil artefacts).
	snap := state.New(t.TempDir() + "/snap.json")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	cfg := PollerConfig{
		ServerURL: srv.URL,
		Token:     "tok",
		ChannelID: "ch",
		Interval:  50 * time.Millisecond,
		Keyword:   "@watchtower",
	}

	RunPoller(ctx, cfg, snap, "", hook, srv.Client())

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

	hook := &captureHook{}
	snap := state.New(t.TempDir() + "/snap.json")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// No token — poller must return immediately without making any HTTP call.
	RunPoller(ctx, PollerConfig{
		ServerURL: srv.URL,
		ChannelID: "ch",
		Interval:  10 * time.Millisecond,
		Keyword:   "@watchtower",
	}, snap, "", hook, srv.Client())

	if called {
		t.Error("poller should not make HTTP calls when Token is missing")
	}
}

// TestDispatchKeywordWithBuildsStatus verifies end-to-end keyword routing for a
// real command.
func TestDispatchKeywordWithBuildsStatus(t *testing.T) {
	hook := &captureHook{}
	artefacts := []buildapi.Artefact{
		{ID: 1, Release: "noble", Version: time.Now().UTC().Format("20060102")},
	}
	if err := Dispatch("@watchtower builds status", artefacts, "", hook, "@watchtower"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(hook.last, "noble") {
		t.Errorf("expected builds status output, got: %s", hook.last)
	}
}
