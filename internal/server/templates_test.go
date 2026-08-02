package server

import "testing"

func TestRunDuration(t *testing.T) {
	t.Parallel()
	const sec = 1_000_000 // epoch microseconds per second
	cases := []struct {
		name              string
		started, finished int64
		want              string
	}{
		{"whole seconds", 1_000 * sec, 1_014 * sec, "14s"},
		{"sub-second rounds down", 1_000 * sec, 1_000*sec + 400_000, "0s"},
		{"rounds up", 1_000 * sec, 1_000*sec + 600_000, "1s"},
		{"zero duration", 1_000 * sec, 1_000 * sec, "0s"},
		{"not started", 0, 1_014 * sec, ""},
		{"not finished", 1_000 * sec, 0, ""},
		{"finish before start", 1_014 * sec, 1_000 * sec, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := runDuration(tc.started, tc.finished); got != tc.want {
				t.Errorf("runDuration(%d, %d) = %q, want %q", tc.started, tc.finished, got, tc.want)
			}
		})
	}
}
