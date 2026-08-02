// Package ytdlp wraps the yt-dlp command-line tool to archive a URL's
// metadata and transcript (no media). All URLs are passed to yt-dlp; unsupported
// URLs will result in a failed step rather than being skipped.
package ytdlp

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/nguyenmp/simplearchive/internal/extractors"
	"github.com/nguyenmp/simplearchive/internal/subproc"
)

// Extractor archives a URL's metadata and transcript via yt-dlp.
type Extractor struct {
	// Bin is the yt-dlp binary path; it defaults to "yt-dlp" when empty. Tests
	// override it with a fake script.
	Bin string
}

// Name returns the extractor registry identifier.
func (Extractor) Name() string { return "ytdlp" }

func (e Extractor) bin() string {
	if e.Bin != "" {
		return e.Bin
	}
	return "yt-dlp"
}

// Run archives url's metadata and transcript into dir. A failed metadata fetch
// returns the steps recorded so far alongside the error (best-effort).
func (e Extractor) Run(ctx context.Context, pageURL, dir string) ([]extractors.Step, error) {
	start := time.Now()
	argv := argv(e.bin(), pageURL)
	_, runErr := subproc.Run(ctx, dir, argv[0], argv[1:]...)
	end := time.Now()

	infoFiles, _ := filepath.Glob(filepath.Join(dir, "*.info.json"))
	vttFiles, _ := filepath.Glob(filepath.Join(dir, "*.vtt"))

	cmdStr := argv
	steps := make([]extractors.Step, 0, 2)

	meta := extractors.Step{Name: "youtube_metadata", Cmd: cmdStr, StartTs: start, EndTs: end}
	if len(infoFiles) > 0 {
		meta.Filename = filepath.Base(infoFiles[0])
		meta.Status = extractors.StatusSucceeded
	} else {
		meta.Status = extractors.StatusFailed
		if runErr != nil {
			meta.Err = fmt.Errorf("ytdlp: %w", runErr)
		} else {
			meta.Err = errors.New("ytdlp: no info.json produced")
		}
	}
	steps = append(steps, meta)

	trans := extractors.Step{Name: "transcript", Cmd: cmdStr, StartTs: start, EndTs: end}
	if len(vttFiles) > 0 {
		trans.Filename = filepath.Base(vttFiles[0])
		trans.Status = extractors.StatusSucceeded
	} else if meta.Status == extractors.StatusSucceeded {
		trans.Status = extractors.StatusSkipped
		trans.Err = errors.New("no transcript available")
	} else {
		trans.Status = extractors.StatusFailed
		trans.Err = meta.Err
	}
	steps = append(steps, trans)

	if meta.Status == extractors.StatusFailed {
		return steps, meta.Err
	}
	return steps, nil
}

// argv builds the yt-dlp invocation recorded in index.json and run by Run. The
// --output template is intentionally just "%(id)s" (no directory): the command
// runs with its working directory set to the snapshot dir (see Run), so yt-dlp
// resolves the relative template against that dir. Embedding the dir in
// --output as well would double-nest the path.
func argv(bin, pageURL string) []string {
	return []string{
		bin,
		"--write-info-json", "--write-subs", "--write-auto-subs", "--sub-langs", "en", "--skip-download",
		"--no-progress", "--no-warnings",
		"--output", "%(id)s",
		pageURL,
	}
}
