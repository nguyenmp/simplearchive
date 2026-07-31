package cli

import (
	"context"
	"fmt"

	"github.com/nguyenmp/simplearchive/internal/ingest"
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

	res, err := ingest.Add(ctx, c.DB, c.ArchiveRoot, url)
	if err != nil {
		c.Logger.Error("add", "url", url, "err", err)
		fmt.Fprintf(c.Stderr, "add: %v\n", err)
		return 1
	}

	c.Logger.Info("add", "url", url, "timestamp", snapshot.Format(res.Timestamp), "dir", res.Dir, "status", "succeeded")
	fmt.Fprintf(c.Stdout, "archived %s url=%q title=%q\n", snapshot.Format(res.Timestamp), url, res.Title)
	return 0
}
