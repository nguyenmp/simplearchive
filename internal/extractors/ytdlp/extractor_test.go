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
	got := argv("yt-dlp", "/tmp/snap", "https://youtu.be/abc")
	want := []string{
		"yt-dlp",
		"--write-info-json", "--write-subs", "--sub-langs", "en", "--skip-download",
		"--no-progress", "--no-warnings",
		"--output", "/tmp/snap/%(id)s",
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
