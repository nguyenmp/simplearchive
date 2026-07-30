package cli

import (
	"context"
	"fmt"

	"github.com/nguyenmp/simplearchive/internal/archive"
	"github.com/nguyenmp/simplearchive/internal/extractors/wget"
	"github.com/nguyenmp/simplearchive/internal/snapshot"
)

// runAdd implements `simplearchive add <url>`.
func (c *CLI) runAdd(ctx context.Context, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(c.Stderr, "usage: simplearchive add <url>")
		return 2
	}
	url := args[0]

	if c.DB == nil {
		fmt.Fprintln(c.Stderr, "add: database not configured")
		return 1
	}

	ts := snapshot.NewTimestamp()
	resolved, err := c.DB.CreateSnapshot(ctx, url, ts)
	if err != nil {
		c.Logger.Error("add: create snapshot", "url", url, "err", err)
		fmt.Fprintf(c.Stderr, "add: failed to create snapshot: %v\n", err)
		return 1
	}

	dir, err := archive.MkdirSnapshot(c.ArchiveRoot, resolved)
	if err != nil {
		c.Logger.Error("add: mkdir snapshot", "url", url, "err", err)
		fmt.Fprintf(c.Stderr, "add: failed to create snapshot dir: %v\n", err)
		return 1
	}

	if _, err := wget.Fetch(ctx, url, dir); err != nil {
		c.Logger.Error("add: wget", "url", url, "err", err)
		fmt.Fprintf(c.Stderr, "add: failed to fetch %q: %v\n", url, err)
		return 1
	}

	c.Logger.Info("add", "url", url, "timestamp", snapshot.Format(resolved), "dir", dir, "status", "pending")
	fmt.Fprintf(c.Stdout, "queued %s dir=%s url=%q status=pending\n", snapshot.Format(resolved), dir, url)
	return 0
}
