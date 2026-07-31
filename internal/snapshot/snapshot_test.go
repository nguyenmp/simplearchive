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
		us   int64
		want string
	}{
		{1728277530511056, "1728277530.511056"},
		{0, "0.000000"},
		{1000000, "1.000000"},
		{1001000, "1.001000"},
		{999000, "0.999000"},
		{1728277530000000, "1728277530.000000"},
	}
	for _, tc := range cases {
		if got := Format(tc.us); got != tc.want {
			t.Errorf("Format(%d) = %q, want %q", tc.us, got, tc.want)
		}
	}
}

func TestFormat_matchesABSampleShape(t *testing.T) {
	t.Parallel()
	// The real ArchiveBox sample dir was "1728277530.511056" (microsecond
	// precision). Our rendering must reproduce it exactly: seconds, a dot,
	// then exactly six digits.
	got := Format(1728277530511056)
	if got != "1728277530.511056" {
		t.Fatalf("Format = %q, want 1728277530.511056", got)
	}
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

func TestParse_roundTripsFormat(t *testing.T) {
	t.Parallel()
	cases := []int64{
		1728277530511056,
		0,
		1000000,
		1001000,
		999000,
		1728277530000000,
		1785453601944000,
	}
	for _, us := range cases {
		got, err := Parse(Format(us))
		if err != nil {
			t.Fatalf("Parse(%q): %v", Format(us), err)
		}
		if got != us {
			t.Errorf("Parse(Format(%d)) = %d, want %d", us, got, us)
		}
	}
}

func TestParse_acceptsShortFraction(t *testing.T) {
	t.Parallel()
	cases := []struct {
		s    string
		want int64
	}{
		{"1.5", 1500000},
		{"1.0005", 1000500},
		{"0.1", 100000},
	}
	for _, tc := range cases {
		got, err := Parse(tc.s)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tc.s, err)
		}
		if got != tc.want {
			t.Errorf("Parse(%q) = %d, want %d", tc.s, got, tc.want)
		}
	}
}

func TestParse_rejectsMalformed(t *testing.T) {
	t.Parallel()
	bad := []string{"", "abc", "1.1234567", "1.2.3", "x.1", "1.x"}
	for _, s := range bad {
		if _, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) returned nil error, want error", s)
		}
	}
}

func TestNewTimestamp_isEpochMicros(t *testing.T) {
	t.Parallel()
	now := time.Now().UnixMicro()
	got := NewTimestamp()
	if got < now-1 || got > now+2000000 {
		t.Fatalf("NewTimestamp = %d, want near %d", got, now)
	}
}
