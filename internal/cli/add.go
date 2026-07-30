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

	ts := snapshot.NewTimestamp()
	c.Logger.Info("add", "url", url, "timestamp", snapshot.Format(ts))
	fmt.Fprintf(c.Stdout, "would archive %q at %s\n", url, snapshot.Format(ts))
	return 0
}
