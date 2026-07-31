package archive

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nguyenmp/simplearchive/internal/snapshot"
)

func TestSnapshotDir_usesABFormat(t *testing.T) {
	t.Parallel()
	got := SnapshotDir("archive", 1728277530511056)
	want := filepath.Join("archive", "1728277530.511056")
	if got != want {
		t.Fatalf("SnapshotDir = %q, want %q", got, want)
	}
}

func TestMkdirSnapshot_createsDir(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "archive")
	dir, err := MkdirSnapshot(root, 1728277530511056)
	if err != nil {
		t.Fatalf("MkdirSnapshot: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("%q is not a directory", dir)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("perm = %v, want 0755", info.Mode().Perm())
	}
}

func TestMkdirSnapshot_idempotent(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "archive")
	if _, err := MkdirSnapshot(root, 1728277530511056); err != nil {
		t.Fatalf("first MkdirSnapshot: %v", err)
	}
	if _, err := MkdirSnapshot(root, 1728277530511056); err != nil {
		t.Fatalf("second MkdirSnapshot: %v", err)
	}
	// Ensure no stray sibling entries were created.
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries under root, want 1", len(entries))
	}
	if entries[0].Name() != snapshot.Format(1728277530511056) {
		t.Fatalf("entry = %q, want %q", entries[0].Name(), snapshot.Format(1728277530511056))
	}
}
