// Package subproc wraps os/exec to run subprocesses with structured start/end
// logging via slog.Default(). Each Run logs a start (debug) and an end (info)
// record carrying the command, exit code, duration, and captured stdout/stderr.
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

// Run executes name with args, capturing stdout and stderr separately. dir may
// be empty to run in the current directory. It logs the start (debug) and end
// (info) of the subprocess. The returned error wraps the underlying exec error
// with the trimmed stderr; Result is always returned alongside it.
func Run(ctx context.Context, dir, name string, args ...string) (Result, error) {
	log := slog.Default()
	cmdStr := strings.Join(append([]string{name}, args...), " ")
	log.Debug("subproc: start", "cmd", cmdStr, "dir", dir)

	start := time.Now()
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	dur := time.Since(start)

	exitCode := 0
	var ee *exec.ExitError
	if errors.As(runErr, &ee) {
		exitCode = ee.ExitCode()
	} else if runErr != nil {
		exitCode = -1
	}

	res := Result{
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
		ExitCode: exitCode,
		Duration: dur,
	}
	log.Info("subproc: done",
		"cmd", cmdStr,
		"exit", exitCode,
		"duration_ms", dur.Milliseconds(),
		"stdout", trunc(res.Stdout),
		"stderr", trunc(res.Stderr),
	)

	if runErr != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return res, fmt.Errorf("subproc %q: %w: %s", name, runErr, msg)
		}
		return res, fmt.Errorf("subproc %q: %w", name, runErr)
	}
	return res, nil
}

// trunc returns b as a string, capped at maxLogBytes with a truncation marker.
func trunc(b []byte) string {
	if len(b) <= maxLogBytes {
		return string(b)
	}
	return string(b[:maxLogBytes]) + "...(truncated)"
}
