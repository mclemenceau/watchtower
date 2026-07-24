package logfetcher_test

import (
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mclemenceau/watchtower/internal/adapters/logfetcher"
	"github.com/mclemenceau/watchtower/internal/domain"
)

func TestHTTPLogFetcher_HappyPath(t *testing.T) {
	const body = "===== Building live filesystems =====\nubuntu-amd64 on Launchpad starting at 2026-07-24 04:24:17\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	f := logfetcher.New(0)
	got, err := f.Fetch(context.Background(), srv.URL+"/log.txt")
	if err != nil {
		t.Fatalf("Fetch returned unexpected error: %v", err)
	}
	if got != body {
		t.Errorf("Fetch content mismatch\n got  %q\n want %q", got, body)
	}
}

func TestHTTPLogFetcher_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	f := logfetcher.New(0)
	_, err := f.Fetch(context.Background(), srv.URL+"/missing.log")
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
	if !errors.Is(err, domain.ErrLogNotFound) {
		t.Errorf("expected errors.Is(err, ErrLogNotFound), got %v", err)
	}
}

func TestHTTPLogFetcher_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := logfetcher.New(0)
	_, err := f.Fetch(context.Background(), srv.URL+"/error.log")
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
	if errors.Is(err, domain.ErrLogNotFound) {
		t.Error("500 error should not be ErrLogNotFound")
	}
}

func TestHTTPLogFetcher_GzipContent(t *testing.T) {
	const content = "ubuntu-amd64 on Launchpad starting at 2026-07-24 04:24:17\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		gz := gzip.NewWriter(w)
		_, _ = gz.Write([]byte(content))
		_ = gz.Close()
	}))
	defer srv.Close()

	f := logfetcher.New(0)
	// Use a .gz suffix URL so the fetcher knows to decompress.
	got, err := f.Fetch(context.Background(), srv.URL+"/log.gz")
	if err != nil {
		t.Fatalf("Fetch gzip returned unexpected error: %v", err)
	}
	if got != content {
		t.Errorf("Fetch gzip content mismatch\n got  %q\n want %q", got, content)
	}
}

func TestMockLogFetcher_ReturnsContent(t *testing.T) {
	m := &logfetcher.MockLogFetcher{Content: "hello log"}
	got, err := m.Fetch(context.Background(), "ignored")
	if err != nil {
		t.Fatalf("MockLogFetcher returned unexpected error: %v", err)
	}
	if got != "hello log" {
		t.Errorf("MockLogFetcher content = %q, want %q", got, "hello log")
	}
}

func TestMockLogFetcher_ReturnsError(t *testing.T) {
	m := &logfetcher.MockLogFetcher{Err: domain.ErrLogNotFound}
	_, err := m.Fetch(context.Background(), "ignored")
	if !errors.Is(err, domain.ErrLogNotFound) {
		t.Errorf("MockLogFetcher should return ErrLogNotFound, got %v", err)
	}
}

// TestMockLogFetcher_Interface verifies the mock satisfies ports.LogFetcher at compile time.
// This is also asserted by the var _ check in client.go, but an explicit test
// ensures test coverage of the mock's exported API.
func TestMockLogFetcher_EmptyContent(t *testing.T) {
	m := &logfetcher.MockLogFetcher{}
	got, err := m.Fetch(context.Background(), "ignored")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("empty mock should return empty string, got %q", got)
	}
}

// TestHTTPLogFetcher_ContextCancelled verifies that a cancelled context aborts the fetch.
func TestHTTPLogFetcher_ContextCancelled(t *testing.T) {
	// A server that blocks forever.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until the request context is cancelled.
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	f := logfetcher.New(0)
	_, err := f.Fetch(ctx, srv.URL+"/slow.log")
	if err == nil {
		t.Fatal("expected error when context is cancelled, got nil")
	}
	if strings.Contains(err.Error(), "unexpected status") {
		t.Errorf("expected a connection/context error, got: %v", err)
	}
}
