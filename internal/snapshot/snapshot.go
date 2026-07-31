// Package snapshot handles snapshot timestamps and their ArchiveBox-compatible
// string representation.
//
// Timestamps are stored in the database as epoch microseconds (int64), matching
// ArchiveBox's on-disk precision. ArchiveBox names snapshot directories and
// index.json "timestamp" fields using a "seconds.microseconds" string (e.g.
// "1728277530.511056"). Format converts between the two with full microsecond
// precision, zero-padding to six fractional digits.
package snapshot

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// NewTimestamp returns the current time as epoch microseconds.
func NewTimestamp() int64 {
	return time.Now().UnixMicro()
}

// Format renders an epoch-microsecond timestamp as ArchiveBox's
// "seconds.microseconds" string (six fractional digits).
//
//	e.g. 1728277530511056 -> "1728277530.511056"
func Format(ts int64) string {
	secs := ts / 1000000
	micros := ts % 1000000
	if micros < 0 {
		secs -= 1
		micros += 1000000
	}
	return fmt.Sprintf("%d.%06d", secs, micros)
}

// Parse decodes an ArchiveBox "seconds.microseconds" timestamp string (as
// written by Format and stored in each snapshot's index.json "timestamp"
// field) back into epoch microseconds. The fractional part is optional and
// may be shorter than six digits; missing digits are treated as zeros.
func Parse(s string) (int64, error) {
	dot := strings.IndexByte(s, '.')
	secsStr, fracStr := s, ""
	if dot >= 0 {
		secsStr, fracStr = s[:dot], s[dot+1:]
	}
	secs, err := strconv.ParseInt(secsStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("snapshot.Parse: %q: %w", s, err)
	}
	if len(fracStr) > 6 {
		return 0, fmt.Errorf("snapshot.Parse: %q: fractional part too long", s)
	}
	// Pad the fractional part to six digits (microseconds). ArchiveBox always
	// writes six digits, but tolerate shorter inputs by treating missing
	// digits as zeros.
	for len(fracStr) < 6 {
		fracStr += "0"
	}
	micros, err := strconv.ParseInt(fracStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("snapshot.Parse: %q: %w", s, err)
	}
	return secs*1000000 + micros, nil
}
