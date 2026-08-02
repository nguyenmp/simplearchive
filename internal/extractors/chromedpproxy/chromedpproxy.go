//go:build chromedp

package chromedpproxy

import (
	"context"

	"github.com/nguyenmp/simplearchive/internal/extractors"
	"github.com/nguyenmp/simplearchive/internal/extractors/chromedp"
)

// Run delegates to chromedp.Extractor with the proxy configured and
// FileSuffix="_proxy" so outputs are written to *_proxy.* filenames in dir,
// leaving the original chromedp outputs untouched.
func (e Extractor) Run(ctx context.Context, pageURL, dir string) ([]extractors.Step, error) {
	inner := chromedp.Extractor{
		Bin:        e.Bin,
		Timeout:    e.Timeout,
		ProxyURL:   e.ProxyURL,
		RemoteURL:  e.RemoteURL,
		FileSuffix: "_proxy",
	}
	return inner.Run(ctx, pageURL, dir)
}
