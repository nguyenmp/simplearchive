package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/nguyenmp/simplearchive/internal/server"
)

// runServe implements `simplearchive serve`: it starts the HTTP server. The
// listen address is taken from the SERVE_ADDR env var (default 127.0.0.1:8080).
func (c *CLI) runServe(ctx context.Context, args []string) int {
	if c.DB == nil {
		fmt.Fprintln(c.Stderr, "serve: database not configured")
		return 1
	}

	addr := os.Getenv("SERVE_ADDR")
	if addr == "" {
		addr = server.DefaultAddr()
	}

	srv := &server.Server{
		Logger:      c.Logger,
		DB:          c.DB,
		ArchiveRoot: c.ArchiveRoot,
		Addr:        addr,
	}
	if err := srv.Run(ctx); err != nil {
		c.Logger.Error("serve: run", "err", err)
		fmt.Fprintf(c.Stderr, "serve: %v\n", err)
		return 1
	}
	return 0
}
