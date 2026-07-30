package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nguyenmp/simplearchive/internal/archive"
	"github.com/nguyenmp/simplearchive/internal/extractors/headers"
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

	htmlPath, err := wget.Fetch(ctx, url, dir)
	if err != nil {
		c.Logger.Error("add: wget", "url", url, "err", err)
		fmt.Fprintf(c.Stderr, "add: failed to fetch %q: %v\n", url, err)
		return 1
	}
	if _, err := wget.FetchFavicon(ctx, url, dir); err != nil {
		c.Logger.Warn("add: favicon", "url", url, "err", err)
	}
	if _, err := headers.Fetch(ctx, url, dir); err != nil {
		c.Logger.Warn("add: headers", "url", url, "err", err)
	}

	// Parse the page title from the fetched HTML for the index and DB row.
	title := ""
	if html, rerr := os.ReadFile(htmlPath); rerr == nil {
		title = archive.ParseTitle(html)
	} else {
		c.Logger.Warn("add: read output.html", "err", rerr)
	}

	outputs := []string{filepath.Base(htmlPath), wget.FaviconFile, headers.OutputFile}
	if err := archive.WriteIndex(archive.IndexData{
		Timestamp: resolved,
		URL:       url,
		Title:     title,
		Dir:       dir,
		Outputs:   outputs,
	}); err != nil {
		c.Logger.Error("add: write index.json", "url", url, "err", err)
		fmt.Fprintf(c.Stderr, "add: failed to write index.json: %v\n", err)
		return 1
	}

	if err := c.DB.UpdateSnapshot(ctx, resolved, title); err != nil {
		c.Logger.Error("add: update snapshot", "url", url, "err", err)
		fmt.Fprintf(c.Stderr, "add: failed to update snapshot: %v\n", err)
		return 1
	}

	c.Logger.Info("add", "url", url, "timestamp", snapshot.Format(resolved), "dir", dir, "status", "succeeded")
	fmt.Fprintf(c.Stdout, "archived %s url=%q title=%q\n", snapshot.Format(resolved), url, title)
	return 0
}
