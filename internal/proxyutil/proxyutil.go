// Package proxyutil provides helpers for SOCKS5 proxy configuration.
package proxyutil

import (
	"fmt"
	"net/http"
	"net/url"
	"os"

	"golang.org/x/net/proxy"
)

// EnvVar returns the SOCKS5 proxy URL from the SOCKS5_PROXY environment
// variable, or an empty string if unset.
func EnvVar() string {
	return os.Getenv("SOCKS5_PROXY")
}

// Transport returns an *http.Transport that routes connections through the
// SOCKS5 proxy at proxyURL. If proxyURL is empty it returns (nil, nil).
// Otherwise it returns an error if the URL or dialer is invalid.
func Transport(proxyURL string) (*http.Transport, error) {
	if proxyURL == "" {
		return nil, nil
	}
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL %q: %w", proxyURL, err)
	}
	dialer, err := proxy.FromURL(u, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("cannot create dialer for %q: %w", proxyURL, err)
	}
	ctxDialer, ok := dialer.(proxy.ContextDialer)
	if !ok {
		return nil, fmt.Errorf("dialer for %q does not implement proxy.ContextDialer", proxyURL)
	}
	return &http.Transport{
		DialContext: ctxDialer.DialContext,
	}, nil
}

// Socks5HostPort strips the socks5:// scheme and returns host:port, or an
// empty string if the URL is not a valid SOCKS5 proxy.
func Socks5HostPort(proxyURL string) string {
	if proxyURL == "" {
		return ""
	}
	u, err := url.Parse(proxyURL)
	if err != nil {
		return ""
	}
	if u.Scheme != "socks5" {
		return ""
	}
	return u.Host
}

// ValidateURL returns an error if proxyURL is non-empty but not a valid
// socks5:// URL.
func ValidateURL(proxyURL string) error {
	if proxyURL == "" {
		return nil
	}
	u, err := url.Parse(proxyURL)
	if err != nil {
		return fmt.Errorf("invalid proxy URL: %w", err)
	}
	if u.Scheme != "socks5" {
		return fmt.Errorf("unsupported proxy scheme %q, expected socks5", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("proxy URL missing host")
	}
	return nil
}
