// Package snapshot handles snapshot timestamps and their ArchiveBox-compatible
// string representation.
//
// Timestamps are stored in the database as epoch milliseconds (int64). On disk,
// ArchiveBox names snapshot directories and index.json "timestamp" fields using
// a "seconds.microseconds" string (e.g. "1728277530.511056"). Format converts
// between the two; the millisecond precision is preserved and zero-padded to
// six fractional digits to match ArchiveBox's format.
package snapshot

import (
	"fmt"
	"time"
)

// NewTimestamp returns the current time as epoch milliseconds.
func NewTimestamp() int64 {
	return time.Now().UnixMilli()
}

// Format renders an epoch-millisecond timestamp as ArchiveBox's
// "seconds.microseconds" string (six fractional digits).
//
//	e.g. 1728277530511 -> "1728277530.511000"
func Format(ts int64) string {
	secs := ts / 1000
	micros := (ts % 1000) * 1000
	if micros < 0 {
		secs -= 1
		micros += 1000000
	}
	return fmt.Sprintf("%d.%06d", secs, micros)
}
