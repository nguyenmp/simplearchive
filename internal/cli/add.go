package cli

import (
	"context"
	"fmt"
)

// runAdd implements `simplearchive add <url>`.
func (c *CLI) runAdd(ctx context.Context, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(c.Stderr, "usage: simplearchive add <url>")
		return 2
	}
	url := args[0]

	// Stub: will be wired up incrementally in later commits.
	fmt.Fprintf(c.Stdout, "would archive %q\n", url)
	return 0
}
