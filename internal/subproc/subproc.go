// Package subproc wraps os/exec to run subprocesses with structured start/end
// logging via slog. Each Run logs a start (debug) and an end (info) record
// carrying the command, exit code, duration, and captured stdout/stderr.
// Use Runner to inject a custom *slog.Logger; the top-level Run function uses
// slog.Default() for backward compatibility.
package subproc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

// maxLogBytes caps how much of stdout/stderr is inlined into the end log
// record so a chatty subprocess cannot swamp the logs.
const maxLogBytes = 4096

// Result is the outcome of a subprocess run. It is always populated, even when
// Run returns an error, so callers can inspect stdout/stderr/exit regardless of
// how the command failed.
type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
	Duration time.Duration
}

// Runner executes subprocesses with an optional injected logger. If Logger is
// nil, Run falls back to slog.Default().
type Runner struct {
	Logger *slog.Logger
}

// Run executes name with args, capturing stdout and stderr separately. dir may
// be empty to run in the current directory. It logs the start (debug) and end
// (info) of the subprocess. The returned error wraps the underlying exec error
// with the trimmed stderr; Result is always returned alongside it.
func (r *Runner) Run(ctx context.Context, dir, name string, args ...string) (Result, error) {
	log := r.Logger
	if log == nil {
		log = slog.Default()
	}
	commandLine := strings.Join(append([]string{name}, args...), " ")
	log.Debug("subproc: start", "cmd", commandLine, "dir", dir)

	start := time.Now()
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		exitCode = exitErr.ExitCode()
	} else if runErr != nil {
		exitCode = -1
	}

	result := Result{
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
		ExitCode: exitCode,
		Duration: duration,
	}
	log.Info("subproc: done",
		"cmd", commandLine,
		"exit", exitCode,
		"duration_ms", duration.Milliseconds(),
		"stdout", trunc(result.Stdout),
		"stderr", trunc(result.Stderr),
	)

	if runErr != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return result, fmt.Errorf("subproc %q: %w: %s", name, runErr, msg)
		}
		return result, fmt.Errorf("subproc %q: %w", name, runErr)
	}
	return result, nil
}

// Run is a convenience wrapper around Runner{}.Run that uses slog.Default()
// for backward compatibility with existing callers.
func Run(ctx context.Context, dir, name string, args ...string) (Result, error) {
	return (&Runner{}).Run(ctx, dir, name, args...)
}

// trunc returns b as a string, capped at maxLogBytes with a truncation marker.
func trunc(data []byte) string {
	if len(data) <= maxLogBytes {
		return string(data)
	}
	return string(data[:maxLogBytes]) + "...(truncated)"
}
