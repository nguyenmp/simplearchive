package ytdlp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nguyenmp/simplearchive/internal/extractors"
)

// fakeBin writes a shell script to a temp file that creates the given output
// files in its working directory and returns the script path. The script exits
// with the given code.
func fakeBin(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-ytdlp")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("write fake bin: %v", err)
	}
	return path
}

func TestExtractor_nonVideoURL_skips(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	steps, err := Extractor{}.Run(context.Background(), "https://example.com/notavideo", dir)
	if !errors.Is(err, extractors.ErrSkipped) {
		t.Fatalf("err = %v, want ErrSkipped", err)
	}
	if steps != nil {
		t.Fatalf("steps = %v, want nil", steps)
	}
}

func TestArgv(t *testing.T) {
	t.Parallel()
	got := argv("yt-dlp", "https://youtu.be/abc")
	want := []string{
		"yt-dlp",
		"--write-info-json", "--write-subs", "--sub-langs", "en", "--skip-download",
		"--no-progress", "--no-warnings",
		"--output", "%(id)s",
		"https://youtu.be/abc",
	}
	if len(got) != len(want) {
		t.Fatalf("argv = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("argv[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestExtractor_writesMetadataAndTranscript(t *testing.T) {
	t.Parallel()
	bin := fakeBin(t, "echo '{\"title\":\"fake\"}' > fake.info.json\necho 'WEBVTT' > fake.en.vtt\n")
	dir := t.TempDir()
	steps, err := Extractor{Bin: bin}.Run(context.Background(), "https://youtu.be/abc", dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("got %d steps, want 2", len(steps))
	}
	if steps[0].Name != "youtube_metadata" || steps[0].Status != extractors.StatusSucceeded || steps[0].Filename != "fake.info.json" {
		t.Errorf("meta = %+v", steps[0])
	}
	if steps[1].Name != "transcript" || steps[1].Status != extractors.StatusSucceeded || steps[1].Filename != "fake.en.vtt" {
		t.Errorf("transcript = %+v", steps[1])
	}
}

func TestExtractor_noTranscript_reportsSkipped(t *testing.T) {
	t.Parallel()
	bin := fakeBin(t, "echo '{\"title\":\"fake\"}' > fake.info.json\n")
	dir := t.TempDir()
	steps, err := Extractor{Bin: bin}.Run(context.Background(), "https://youtu.be/abc", dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if steps[0].Status != extractors.StatusSucceeded {
		t.Errorf("meta status = %q, want succeeded", steps[0].Status)
	}
	if steps[1].Status != extractors.StatusSkipped {
		t.Errorf("transcript status = %q, want skipped", steps[1].Status)
	}
}

// TestExtractor_relativeDir_noNesting guards against the path-nesting bug
// where setting both cmd.Dir and a dir-prefixed --output caused yt-dlp to write
// to <dir>/<dir>/<id>.info.json. It runs with a relative snapshot dir, so the
// fake binary's cwd (cmd.Dir) is relative; the output must land directly under
// the snapshot dir. Not parallel: it changes the process working directory.
func TestExtractor_relativeDir_noNesting(t *testing.T) {
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(orig) }()

	root := t.TempDir()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	const rel = "snap"
	if err := os.Mkdir(rel, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	bin := fakeBin(t, "echo '{\"title\":\"fake\"}' > fake.info.json\n")

	steps, err := Extractor{Bin: bin}.Run(context.Background(), "https://youtu.be/abc", rel)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if steps[0].Status != extractors.StatusSucceeded || steps[0].Filename != "fake.info.json" {
		t.Fatalf("meta = %+v, want succeeded fake.info.json", steps[0])
	}
	// Output must be directly under the snapshot dir, not nested.
	if _, err := os.Stat(filepath.Join(rel, "fake.info.json")); err != nil {
		t.Errorf("file missing under %q: %v", rel, err)
	}
	if _, err := os.Stat(filepath.Join(rel, rel, "fake.info.json")); err == nil {
		t.Errorf("file nested under %q/%q (path-nesting bug regressed)", rel, rel)
	}
}

func TestExtractor_metadataFailed_returnsError(t *testing.T) {
	t.Parallel()
	bin := fakeBin(t, "exit 1\n")
	dir := t.TempDir()
	steps, err := Extractor{Bin: bin}.Run(context.Background(), "https://youtu.be/abc", dir)
	if err == nil {
		t.Fatal("Run on failing yt-dlp returned nil error")
	}
	if len(steps) != 2 {
		t.Fatalf("got %d steps, want 2", len(steps))
	}
	if steps[0].Status != extractors.StatusFailed || steps[0].Err == nil {
		t.Errorf("meta = %+v, want failed with err", steps[0])
	}
	if steps[1].Status != extractors.StatusFailed {
		t.Errorf("transcript = %+v, want failed", steps[1])
	}
}
