package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nguyenmp/simplearchive/internal/meta"
	"github.com/nguyenmp/simplearchive/internal/snapshot"
)

func TestRun_noArgs_printsUsage(t *testing.T) {
	t.Parallel()
	c := &CLI{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
	got := c.Run(context.Background(), nil)
	if got != 2 {
		t.Fatalf("exit = %d, want 2", got)
	}
	if !strings.Contains(c.Stderr.(*bytes.Buffer).String(), "usage:") {
		t.Fatalf("stderr missing usage: %q", c.Stderr)
	}
}

func TestRun_unknownCommand(t *testing.T) {
	t.Parallel()
	c := &CLI{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
	got := c.Run(context.Background(), []string{"frobnicate"})
	if got != 2 {
		t.Fatalf("exit = %d, want 2", got)
	}
	if !strings.Contains(c.Stderr.(*bytes.Buffer).String(), `unknown command "frobnicate"`) {
		t.Fatalf("stderr missing unknown-command message: %q", c.Stderr)
	}
}

func TestRun_addNoDB_reportsError(t *testing.T) {
	t.Parallel()
	c := &CLI{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
	got := c.Run(context.Background(), []string{"add", "https://example.com"})
	if got != 1 {
		t.Fatalf("exit = %d, want 1", got)
	}
	if !strings.Contains(c.Stderr.(*bytes.Buffer).String(), "database not configured") {
		t.Fatalf("stderr = %q, want database-not-configured", c.Stderr)
	}
}

func TestRun_add_createsPendingRow(t *testing.T) {
	t.Parallel()
	db, err := meta.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("meta.Open: %v", err)
	}
	defer db.Close()

	out := &bytes.Buffer{}
	root := filepath.Join(t.TempDir(), "archive")
	c := &CLI{Stdout: out, Stderr: &bytes.Buffer{}, DB: db, ArchiveRoot: root}
	got := c.Run(context.Background(), []string{"add", "https://example.com"})
	if got != 0 {
		t.Fatalf("exit = %d, want 0", got)
	}
	if !strings.Contains(out.String(), "status=pending") {
		t.Fatalf("stdout = %q, want status=pending", out.String())
	}

	var status, tsStr string
	if err := db.QueryRow("SELECT status, printf('%d', timestamp) FROM snapshots WHERE url = ?", "https://example.com").Scan(&status, &tsStr); err != nil {
		t.Fatalf("query snapshot: %v", err)
	}
	if status != "pending" {
		t.Fatalf("status = %q, want pending", status)
	}

	// The snapshot directory must exist under the archive root.
	var ts int64
	if _, err := fmt.Sscan(tsStr, &ts); err != nil {
		t.Fatalf("parse timestamp %q: %v", tsStr, err)
	}
	dir := filepath.Join(root, snapshot.Format(ts))
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("snapshot dir %q not created: %v", dir, err)
	}
}

func TestRun_addNoURL_usageError(t *testing.T) {
	t.Parallel()
	c := &CLI{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
	got := c.Run(context.Background(), []string{"add"})
	if got != 2 {
		t.Fatalf("exit = %d, want 2", got)
	}
	if !strings.Contains(c.Stderr.(*bytes.Buffer).String(), "usage: simplearchive add <url>") {
		t.Fatalf("stderr missing add usage: %q", c.Stderr)
	}
}
