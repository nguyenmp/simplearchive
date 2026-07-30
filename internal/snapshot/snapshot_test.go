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

func TestParse_roundTripsFormat(t *testing.T) {
	t.Parallel()
	cases := []int64{
		1728277530511,
		0,
		1000,
		1001,
		999,
		1728277530000,
		1785453601944,
	}
	for _, ms := range cases {
		got, err := Parse(Format(ms))
		if err != nil {
			t.Fatalf("Parse(%q): %v", Format(ms), err)
		}
		if got != ms {
			t.Errorf("Parse(Format(%d)) = %d, want %d", ms, got, ms)
		}
	}
}

func TestParse_acceptsShortFraction(t *testing.T) {
	t.Parallel()
	cases := []struct {
		s    string
		want int64
	}{
		{"1.5", 1500},
		{"1.0005", 1000}, // 1.000500 -> micros 500 -> /1000 = 0 (ms precision)
		{"0.1", 100},
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

func TestNewTimestamp_isEpochMs(t *testing.T) {
	t.Parallel()
	now := time.Now().UnixMilli()
	got := NewTimestamp()
	if got < now-1 || got > now+2000 {
		t.Fatalf("NewTimestamp = %d, want near %d", got, now)
	}
}
