package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nguyenmp/simplearchive/internal/archive"
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

func TestRun_importNoDB_reportsError(t *testing.T) {
	t.Parallel()
	c := &CLI{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
	got := c.Run(context.Background(), []string{"import"})
	if got != 1 {
		t.Fatalf("exit = %d, want 1", got)
	}
	if !strings.Contains(c.Stderr.(*bytes.Buffer).String(), "database not configured") {
		t.Fatalf("stderr = %q, want database-not-configured", c.Stderr)
	}
}

func TestRun_import_loadsSnapshotsIntoDB(t *testing.T) {
	t.Parallel()
	db, err := meta.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("meta.Open: %v", err)
	}
	defer db.Close()

	root := filepath.Join(t.TempDir(), "archive")
	// Two snapshots written out of order; import must load both.
	for _, data := range []archive.IndexData{
		{Timestamp: 1700000000000002, URL: "https://b.example.com", Title: "B", Outputs: []string{"output.html"}},
		{Timestamp: 1700000000000001, URL: "https://a.example.com", Title: "A", Outputs: []string{"output.html"}},
	} {
		dir := filepath.Join(root, snapshot.Format(data.Timestamp))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		data.Dir = dir
		if err := archive.WriteIndex(data); err != nil {
			t.Fatalf("WriteIndex: %v", err)
		}
	}

	out := &bytes.Buffer{}
	c := &CLI{Stdout: out, Stderr: &bytes.Buffer{}, DB: db, ArchiveRoot: root}
	got := c.Run(context.Background(), []string{"import"})
	if got != 0 {
		t.Fatalf("exit = %d, want 0", got)
	}
	if !strings.Contains(out.String(), "imported 2 snapshots") {
		t.Fatalf("stdout = %q, want imported summary", out.String())
	}

	var n int
	if err := db.QueryRow("SELECT count(*) FROM snapshots").Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Fatalf("snapshot count = %d, want 2", n)
	}
	var urlA, urlB string
	if err := db.QueryRow("SELECT url FROM snapshots WHERE timestamp = ?", 1700000000000001).Scan(&urlA); err != nil {
		t.Fatalf("query A: %v", err)
	}
	if err := db.QueryRow("SELECT url FROM snapshots WHERE timestamp = ?", 1700000000000002).Scan(&urlB); err != nil {
		t.Fatalf("query B: %v", err)
	}
	if urlA != "https://a.example.com" || urlB != "https://b.example.com" {
		t.Errorf("urls = %q, %q", urlA, urlB)
	}
}

func TestRun_import_isIdempotent(t *testing.T) {
	t.Parallel()
	db, err := meta.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("meta.Open: %v", err)
	}
	defer db.Close()

	root := filepath.Join(t.TempDir(), "archive")
	dir := filepath.Join(root, snapshot.Format(1700000000000000))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := archive.WriteIndex(archive.IndexData{
		Timestamp: 1700000000000000,
		URL:       "https://example.com",
		Title:     "Example",
		Dir:       dir,
		Outputs:   []string{"output.html"},
	}); err != nil {
		t.Fatalf("WriteIndex: %v", err)
	}

	c := &CLI{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, DB: db, ArchiveRoot: root}
	if got := c.Run(context.Background(), []string{"import"}); got != 0 {
		t.Fatalf("first import exit = %d, want 0", got)
	}
	if got := c.Run(context.Background(), []string{"import"}); got != 0 {
		t.Fatalf("second import exit = %d, want 0", got)
	}

	var n int
	if err := db.QueryRow("SELECT count(*) FROM snapshots").Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("snapshot count = %d, want 1 after re-import", n)
	}
}
