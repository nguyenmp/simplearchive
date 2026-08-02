//go:build !chromedp

package chromedp

import (
	"context"

	"github.com/nguyenmp/simplearchive/internal/extractors"
)

// Run always returns ErrSkipped when the chromedp build tag is not enabled.
func (Extractor) Run(context.Context, string, string) ([]extractors.Step, error) {
	return nil, extractors.ErrSkipped
}
