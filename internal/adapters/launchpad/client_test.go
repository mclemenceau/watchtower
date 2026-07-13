package launchpad_test

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mclemenceau/watchtower/internal/adapters/launchpad"
	"github.com/mclemenceau/watchtower/internal/ports"
)

// Compile-time check that HTTPLaunchpadSource satisfies ports.LaunchpadSource.
var _ ports.LaunchpadSource = (*launchpad.HTTPLaunchpadSource)(nil)

// Compile-time check that MockLaunchpadSource satisfies ports.LaunchpadSource.
var _ ports.LaunchpadSource = (*launchpad.MockLaunchpadSource)(nil)

func TestHTTPLaunchpadSource_ReturnsLogURL(t *testing.T) {
	wantURL := "https://launchpadlibrarian.net/123456/buildlog.txt.gz"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		payload := map[string]interface{}{"build_log_url": wantURL}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	src := launchpad.NewHTTPLaunchpadSourceWithClient(srv.Client(), srv.URL)
	// Use a fake launchpad.net path — the client will redirect to srv.URL base.
	gotURL, err := src.FetchBuildLogURL(context.Background(), "https://launchpad.net/~team/+livefs/ubuntu/stonking/ubuntu/+build/1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotURL != wantURL {
		t.Errorf("FetchBuildLogURL = %q, want %q", gotURL, wantURL)
	}
}

func TestHTTPLaunchpadSource_NullBuildLog(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		payload := map[string]interface{}{"build_log_url": nil}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	src := launchpad.NewHTTPLaunchpadSourceWithClient(srv.Client(), srv.URL)
	gotURL, err := src.FetchBuildLogURL(context.Background(), "https://launchpad.net/~team/+livefs/ubuntu/stonking/ubuntu/+build/1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotURL != "" {
		t.Errorf("FetchBuildLogURL = %q, want empty string for null build_log", gotURL)
	}
}

func TestHTTPLaunchpadSource_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	src := launchpad.NewHTTPLaunchpadSourceWithClient(srv.Client(), srv.URL)
	_, err := src.FetchBuildLogURL(context.Background(), "https://launchpad.net/~team/+livefs/ubuntu/stonking/ubuntu/+build/1")
	if err == nil {
		t.Error("expected error for 404 response, got nil")
	}
}

func TestHTTPLaunchpadSource_NotLaunchpadURL(t *testing.T) {
	src := launchpad.NewHTTPLaunchpadSource()
	_, err := src.FetchBuildLogURL(context.Background(), "https://example.com/not-launchpad")
	if err == nil {
		t.Error("expected error for non-launchpad URL, got nil")
	}
}

func TestMockLaunchpadSource_ReturnsConfiguredValues(t *testing.T) {
	mock := &launchpad.MockLaunchpadSource{URL: "https://example.com/log.txt.gz"}
	got, err := mock.FetchBuildLogURL(context.Background(), "ignored")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "https://example.com/log.txt.gz" {
		t.Errorf("MockLaunchpadSource returned %q, want %q", got, "https://example.com/log.txt.gz")
	}
}

// TestGzipDecompression verifies that a gzip-compressed response body can be
// decoded — this is a helper used to validate the gzip integration path.
func TestGzipDecompression(t *testing.T) {
	wantContent := "line 1\nline 2\nfinal line\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-gzip")
		gz := gzip.NewWriter(w)
		_, _ = gz.Write([]byte(wantContent))
		_ = gz.Close()
	}))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	gr, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer gr.Close() //nolint:errcheck

	var sb strings.Builder
	buf := make([]byte, 256)
	for {
		n, readErr := gr.Read(buf)
		if n > 0 {
			sb.Write(buf[:n])
		}
		if readErr != nil {
			break
		}
	}
	if sb.String() != wantContent {
		t.Errorf("decompressed content = %q, want %q", sb.String(), wantContent)
	}
}
