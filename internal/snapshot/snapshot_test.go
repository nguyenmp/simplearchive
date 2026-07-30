package snapshot

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestFormat_roundTripsABShape(t *testing.T) {
	t.Parallel()
	cases := []struct {
		ms   int64
		want string
	}{
		{1728277530511, "1728277530.511000"},
		{0, "0.000000"},
		{1000, "1.000000"},
		{1001, "1.001000"},
		{999, "0.999000"},
		{1728277530000, "1728277530.000000"},
	}
	for _, tc := range cases {
		if got := Format(tc.ms); got != tc.want {
			t.Errorf("Format(%d) = %q, want %q", tc.ms, got, tc.want)
		}
	}
}

func TestFormat_matchesABSampleShape(t *testing.T) {
	t.Parallel()
	// The real ArchiveBox sample dir was "1728277530.511056" (microsecond
	// precision). Our ms-precision rendering must be a valid shape: seconds,
	// a dot, then exactly six digits.
	got := Format(1728277530511)
	parts := strings.SplitN(got, ".", 2)
	if len(parts) != 2 {
		t.Fatalf("Format = %q, want a dot-separated value", got)
	}
	if _, err := strconv.ParseInt(parts[0], 10, 64); err != nil {
		t.Fatalf("seconds part %q not an int: %v", parts[0], err)
	}
	if len(parts[1]) != 6 {
		t.Fatalf("fractional part %q has %d digits, want 6", parts[1], len(parts[1]))
	}
}

func TestNewTimestamp_isEpochMs(t *testing.T) {
	t.Parallel()
	now := time.Now().UnixMilli()
	got := NewTimestamp()
	if got < now-1 || got > now+2000 {
		t.Fatalf("NewTimestamp = %d, want near %d", got, now)
	}
}
