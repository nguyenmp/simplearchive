package subproc

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestRun_capturesStdoutAndStderr(t *testing.T) {
	t.Parallel()
	res, err := Run(context.Background(), "", "sh", "-c", "echo out; echo err 1>&2")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("exit = %d, want 0", res.ExitCode)
	}
	if strings.TrimSpace(string(res.Stdout)) != "out" {
		t.Errorf("stdout = %q, want out", res.Stdout)
	}
	if strings.TrimSpace(string(res.Stderr)) != "err" {
		t.Errorf("stderr = %q, want err", res.Stderr)
	}
	if res.Duration <= 0 {
		t.Error("duration not set")
	}
}

func TestRun_nonZeroExit_returnsError(t *testing.T) {
	t.Parallel()
	res, err := Run(context.Background(), "", "sh", "-c", "echo boom 1>&2; exit 3")
	if err == nil {
		t.Fatal("Run on exit 3 returned nil error")
	}
	if res.ExitCode != 3 {
		t.Errorf("exit = %d, want 3", res.ExitCode)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v, want it to contain stderr", err)
	}
	if strings.TrimSpace(string(res.Stderr)) != "boom" {
		t.Errorf("stderr = %q, want boom", res.Stderr)
	}
}

func TestRun_dirSetsWorkingDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	res, err := Run(context.Background(), dir, "sh", "-c", "pwd")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.TrimSpace(string(res.Stdout)) != dir {
		t.Errorf("stdout = %q, want %q", res.Stdout, dir)
	}
}

func TestRun_binaryNotFound_returnsError(t *testing.T) {
	t.Parallel()
	res, err := Run(context.Background(), "", "no-such-binary-xyz")
	if err == nil {
		t.Fatal("Run on missing binary returned nil error")
	}
	if res.ExitCode != -1 {
		t.Errorf("exit = %d, want -1 for launch failure", res.ExitCode)
	}
}

func TestRun_contextCancel_returnsError(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Run(ctx, "", "sh", "-c", "sleep 10")
	if err == nil {
		t.Fatal("Run with cancelled context returned nil error")
	}
	if execErr := (&exec.ExitError{}); execErr == nil {
		t.Error("sanity: nil ExitError reference")
	}
}
