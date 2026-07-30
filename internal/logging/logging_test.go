package logging

import (
	"log/slog"
	"testing"
)

func TestParseLevel(t *testing.T) {
	for _, tc := range []struct {
		env    string
		expect slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"", slog.LevelInfo},
		{"bogus", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
	} {
		if got := parseLevel(tc.env); got != tc.expect {
			t.Errorf("parseLevel(%q) = %v, want %v", tc.env, got, tc.expect)
		}
	}
}

func TestNew_returnsJSONHandlerAtParsedLevel(t *testing.T) {
	l := New("debug")
	if !l.Enabled(nil, slog.LevelDebug) {
		t.Error("debug-enabled logger should be Enabled at Debug level")
	}
	l = New("error")
	if l.Enabled(nil, slog.LevelInfo) {
		t.Error("error-level logger should not be Enabled at Info level")
	}
}