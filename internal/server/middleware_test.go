package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nguyenmp/simplearchive/internal/meta"
)

func TestRequestLogger_logsAndPassesThrough(t *testing.T) {
	t.Parallel()

	// Capture slog output in a buffer.
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	db, err := meta.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("meta.Open: %v", err)
	}
	defer db.Close()

	s := &Server{Logger: logger, DB: db}
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	// The request log line should be present.
	var found bool
	for _, line := range bytes.Split(buf.Bytes(), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if msg, _ := entry["msg"].(string); msg == "http" {
			if entry["method"] != http.MethodGet {
				t.Errorf("log method = %v, want %s", entry["method"], http.MethodGet)
			}
			if entry["path"] != "/healthz" {
				t.Errorf("log path = %v, want /healthz", entry["path"])
			}
			if entry["status"] != float64(http.StatusOK) {
				t.Errorf("log status = %v, want %d", entry["status"], http.StatusOK)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("no http request log line in: %s", buf.String())
	}
}
