// Package wget wraps the wget command-line tool to fetch a URL into a snapshot
// directory.
package wget

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
)

// OutputFile is the filename wget writes the page body to.
const OutputFile = "output.html"

// Fetch downloads url and writes it to dir/output.html using wget. It returns
// the path of the written file.
func Fetch(ctx context.Context, url, dir string) (string, error) {
	out := filepath.Join(dir, OutputFile)
	cmd := exec.CommandContext(ctx, "wget",
		"--no-verbose",
		"--output-document="+out,
		url,
	)
	if stderr, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("wget.Fetch: %w: %s", err, stderr)
	}
	return out, nil
}
