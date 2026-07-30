package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/nguyenmp/simplearchive/internal/meta"
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
	c := &CLI{Stdout: out, Stderr: &bytes.Buffer{}, DB: db}
	got := c.Run(context.Background(), []string{"add", "https://example.com"})
	if got != 0 {
		t.Fatalf("exit = %d, want 0", got)
	}
	if !strings.Contains(out.String(), "status=pending") {
		t.Fatalf("stdout = %q, want status=pending", out.String())
	}

	var status string
	if err := db.QueryRow("SELECT status FROM snapshots WHERE url = ?", "https://example.com").Scan(&status); err != nil {
		t.Fatalf("query snapshot: %v", err)
	}
	if status != "pending" {
		t.Fatalf("status = %q, want pending", status)
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
