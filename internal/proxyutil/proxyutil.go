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
// SOCKS5 proxy at proxyURL. If proxyURL is empty or invalid, it returns nil.
func Transport(proxyURL string) *http.Transport {
	if proxyURL == "" {
		return nil
	}
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil
	}
	dialer, err := proxy.FromURL(u, proxy.Direct)
	if err != nil {
		return nil
	}
	ctxDialer, ok := dialer.(proxy.ContextDialer)
	if !ok {
		return nil
	}
	return &http.Transport{
		DialContext: ctxDialer.DialContext,
	}
}

// CommandFlag returns the --proxy argument value for subcommands that speak
// URLs natively (yt-dlp, curl --proxy, etc.). If proxyURL is empty, it
// returns an empty string.
func CommandFlag(proxyURL string) string {
	return proxyURL
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
