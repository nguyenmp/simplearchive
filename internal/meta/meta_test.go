package meta

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpen_memory(t *testing.T) {
	t.Parallel()
	db, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open(\"memory:\") = %v", err)
	}
	defer db.Close()

	if db.DB == nil {
		t.Fatal("opened DB has nil sql.DB")
	}
}

func TestOpen_fileAndReopen(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "meta.db")

	for i := 0; i < 2; i++ {
		db, err := Open(context.Background(), path)
		if err != nil {
			t.Fatalf("open %d: %v", i+1, err)
		}
		if err := db.Close(); err != nil {
			t.Fatalf("close %d: %v", i+1, err)
		}
	}
}

func TestOpen_invalidPath(t *testing.T) {
	t.Parallel()
	// A path under a nonexistent directory cannot be created.
	_, err := Open(context.Background(), "/nonexistent/dir/meta.db")
	if err == nil {
		t.Fatal("expected error opening database under nonexistent directory")
	}
}

func TestClose_nilSafe(t *testing.T) {
	t.Parallel()
	var d *DB
	if err := d.Close(); err != nil {
		t.Fatalf("(*DB).Close on nil receiver = %v", err)
	}
}