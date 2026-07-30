// Package archive manages the on-disk archive/ directory tree, mirroring
// ArchiveBox's layout: archive/{timestamp}/ holds a snapshot's outputs.
package archive

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nguyenmp/simplearchive/internal/snapshot"
)

// SnapshotDir returns the path to a snapshot's directory under root, formatted
// with ArchiveBox's "seconds.microseconds" naming.
func SnapshotDir(root string, ts int64) string {
	return filepath.Join(root, snapshot.Format(ts))
}

// MkdirSnapshot creates a snapshot's directory under root (idempotent) and
// returns its path. The directory is created with mode 0755.
func MkdirSnapshot(root string, ts int64) (string, error) {
	dir := SnapshotDir(root, ts)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("archive.MkdirSnapshot: mkdir %q: %w", dir, err)
	}
	return dir, nil
}
