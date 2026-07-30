package cli

import (
	"context"
	"fmt"

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
	c.Logger.Info("add", "url", url, "timestamp", snapshot.Format(resolved), "status", "pending")
	fmt.Fprintf(c.Stdout, "queued %s url=%q status=pending\n", snapshot.Format(resolved), url)
	return 0
}
