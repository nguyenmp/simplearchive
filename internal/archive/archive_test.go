package archive

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nguyenmp/simplearchive/internal/extractors"
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

func TestRemoveSnapshot_removesDir(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "archive")
	const ts int64 = 1728277530511056
	dir, err := MkdirSnapshot(root, ts)
	if err != nil {
		t.Fatalf("MkdirSnapshot: %v", err)
	}
	// Create a file inside so RemoveAll has real work to do.
	f := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(f, []byte("hello"), extractors.DefaultFilePerm); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := RemoveSnapshot(root, ts); err != nil {
		t.Fatalf("RemoveSnapshot: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("dir %q still exists after RemoveSnapshot", dir)
	}
}

func TestRemoveSnapshot_idempotent(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "archive")
	const ts int64 = 1728277530511056
	// Removing a nonexistent directory should not error.
	if err := RemoveSnapshot(root, ts); err != nil {
		t.Fatalf("RemoveSnapshot on missing dir: %v", err)
	}
}
