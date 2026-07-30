package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
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

func TestRun_addStub_printsWouldArchive(t *testing.T) {
	t.Parallel()
	out := &bytes.Buffer{}
	c := &CLI{Stdout: out, Stderr: &bytes.Buffer{}}
	got := c.Run(context.Background(), []string{"add", "https://example.com"})
	if got != 0 {
		t.Fatalf("exit = %d, want 0", got)
	}
	if !strings.Contains(out.String(), `would archive "https://example.com"`) {
		t.Fatalf("stdout = %q, want would-archive line", out.String())
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
