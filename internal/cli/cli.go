package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/nguyenmp/simplearchive/internal/meta"
)

// CLI holds shared dependencies for all subcommands.
type CLI struct {
	Stdout      io.Writer
	Stderr      io.Writer
	Logger      *slog.Logger
	DB          *meta.DB
	ArchiveRoot string
}

// Run dispatches a subcommand based on args[0]. It returns the process exit code.
func (c *CLI) Run(ctx context.Context, args []string) int {
	if c.Stdout == nil {
		c.Stdout = os.Stdout
	}
	if c.Stderr == nil {
		c.Stderr = os.Stderr
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	if c.ArchiveRoot == "" {
		c.ArchiveRoot = "archive"
	}

	if len(args) < 1 {
		c.printUsage()
		return 2
	}

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "import":
		return c.runImport(ctx, rest)
	case "serve":
		return c.runServe(ctx, rest)
	case "help", "-h", "--help":
		c.printUsage()
		return 0
	default:
		fmt.Fprintf(c.Stderr, "unknown command %q\n", cmd)
		c.printUsage()
		return 2
	}
}

func (c *CLI) printUsage() {
	fmt.Fprintln(c.Stderr, "usage: simplearchive <command> [args]")
	fmt.Fprintln(c.Stderr, "commands:")
	fmt.Fprintln(c.Stderr, "  import      scan archive/ and load snapshots into meta.db")
	fmt.Fprintln(c.Stderr, "  serve       run the HTTP server (SERVE_ADDR, default 127.0.0.1:8080)")
}
