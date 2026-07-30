package cli

import (
	"context"
	"fmt"

	"github.com/nguyenmp/simplearchive/internal/archive"
	"github.com/nguyenmp/simplearchive/internal/meta"
)

// runImport implements `simplearchive import`: it scans ArchiveRoot for
// per-snapshot index.json files and upserts each into meta.db inside a single
// transaction so existing snapshots become queryable. Safe to re-run.
func (c *CLI) runImport(ctx context.Context, args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(c.Stderr, "usage: simplearchive import")
		return 2
	}
	if c.DB == nil {
		fmt.Fprintln(c.Stderr, "import: database not configured")
		return 1
	}

	entries, err := archive.Scan(c.ArchiveRoot)
	if err != nil {
		c.Logger.Error("import: scan", "root", c.ArchiveRoot, "err", err)
		fmt.Fprintf(c.Stderr, "import: failed to scan %q: %v\n", c.ArchiveRoot, err)
		return 1
	}

	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		c.Logger.Error("import: begin tx", "err", err)
		fmt.Fprintf(c.Stderr, "import: failed to begin transaction: %v\n", err)
		return 1
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()

	for _, e := range entries {
		if err := meta.UpsertSnapshotTx(ctx, tx, e); err != nil {
			c.Logger.Error("import: upsert", "timestamp", e.Timestamp, "url", e.URL, "err", err)
			fmt.Fprintf(c.Stderr, "import: failed to upsert snapshot %d (%s): %v\n", e.Timestamp, e.URL, err)
			return 1
		}
	}
	if err := tx.Commit(); err != nil {
		c.Logger.Error("import: commit", "err", err)
		fmt.Fprintf(c.Stderr, "import: failed to commit: %v\n", err)
		return 1
	}
	committed = true

	c.Logger.Info("import", "root", c.ArchiveRoot, "count", len(entries))
	fmt.Fprintf(c.Stdout, "imported %d snapshots from %s\n", len(entries), c.ArchiveRoot)
	return 0
}
