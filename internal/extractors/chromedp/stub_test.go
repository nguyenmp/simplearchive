//go:build !chromedp

package chromedp

import (
	"context"
	"errors"
	"testing"

	"github.com/nguyenmp/simplearchive/internal/extractors"
)

func TestExtractor_skipsWithoutBuildTag(t *testing.T) {
	t.Parallel()
	steps, err := Extractor{}.Run(context.Background(), "https://example.com", t.TempDir())
	if !errors.Is(err, extractors.ErrSkipped) {
		t.Fatalf("err = %v, want ErrSkipped", err)
	}
	if steps != nil {
		t.Fatalf("steps = %v, want nil", steps)
	}
}
