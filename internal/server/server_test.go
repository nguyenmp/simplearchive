package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nguyenmp/simplearchive/internal/meta"
)

// newTestDB returns an in-memory SQLite database suitable for tests.
func newTestDB(t *testing.T) *meta.DB {
	t.Helper()
	db, err := meta.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("meta.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestHandleHealthz(t *testing.T) {
	t.Parallel()
	s := &Server{DB: newTestDB(t)}
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("content-type = %q, want application/json", got)
	}
	if body := rec.Body.String(); body != `{"ok":true}` {
		t.Errorf("body = %q, want {\"ok\":true}", body)
	}
}

func TestServeStatic_tailwindCSS(t *testing.T) {
	t.Parallel()
	s := &Server{DB: newTestDB(t)}
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/static/tailwind.css", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.Len(); got == 0 {
		t.Fatal("tailwind.css body is empty")
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/css; charset=utf-8" {
		t.Errorf("content-type = %q, want text/css", ct)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("nosniff = %q, want nosniff", got)
	}
}

func TestServeStatic_missingFile(t *testing.T) {
	t.Parallel()
	s := &Server{DB: newTestDB(t)}
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/static/does-not-exist.css", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestRun_noDB_returnsError(t *testing.T) {
	t.Parallel()
	s := &Server{}
	if err := s.Run(context.Background()); err == nil {
		t.Fatal("Run with nil DB returned nil error")
	}
}

// TestRun_servesOverListener starts a real server on an ephemeral port and
// verifies it answers /healthz over TCP, then shuts down cleanly.
func TestRun_servesOverListener(t *testing.T) {
	t.Parallel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	s := &Server{
		DB:       newTestDB(t),
		Listener: ln,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	// Give the goroutine a moment to start serving.
	addr := ln.Addr().String()
	var resp *http.Response
	for i := 0; i < 50; i++ {
		resp, err = http.Get("http://" + addr + "/healthz")
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}
