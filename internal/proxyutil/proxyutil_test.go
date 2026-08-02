package proxyutil

import (
	"testing"
)

func TestSocks5HostPort(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"socks5://tor-socks-proxy:9150", "tor-socks-proxy:9150"},
		{"socks5://1.2.3.4:1080", "1.2.3.4:1080"},
		{"http://proxy:8080", ""},
		{"bad-url", ""},
	}
	for _, c := range cases {
		got := Socks5HostPort(c.input)
		if got != c.want {
			t.Errorf("Socks5HostPort(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestEnvVar(t *testing.T) {
	t.Parallel()
	// Just make sure it doesn't panic. We can't reliably set env vars in parallel tests.
	_ = EnvVar()
}
