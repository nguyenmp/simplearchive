package proxyutil

import (
	"errors"
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

func TestTryDirectThenProxy(t *testing.T) {
	t.Parallel()
	errDirect := errors.New("direct failed")
	errProxy := errors.New("proxy failed")

	t.Run("direct succeeds", func(t *testing.T) {
		t.Parallel()
		val, usedProxy, err := TryDirectThenProxy(
			func() (string, error) { return "ok", nil },
			func() (string, error) { return "proxy", nil },
			"socks5://host:1",
		)
		if err != nil || val != "ok" || usedProxy {
			t.Fatalf("got %q %v %v", val, usedProxy, err)
		}
	})

	t.Run("direct fails proxy succeeds", func(t *testing.T) {
		t.Parallel()
		val, usedProxy, err := TryDirectThenProxy(
			func() (string, error) { return "", errDirect },
			func() (string, error) { return "proxy", nil },
			"socks5://host:1",
		)
		if err != nil || val != "proxy" || !usedProxy {
			t.Fatalf("got %q %v %v", val, usedProxy, err)
		}
	})

	t.Run("both fail", func(t *testing.T) {
		t.Parallel()
		val, usedProxy, err := TryDirectThenProxy(
			func() (string, error) { return "", errDirect },
			func() (string, error) { return "", errProxy },
			"socks5://host:1",
		)
		if err == nil || val != "" || !usedProxy {
			t.Fatalf("got %q %v %v", val, usedProxy, err)
		}
		if !errors.Is(err, errDirect) || !errors.Is(err, errProxy) {
			t.Fatalf("combined error should wrap both: %v", err)
		}
	})

	t.Run("direct fails no proxy", func(t *testing.T) {
		t.Parallel()
		val, usedProxy, err := TryDirectThenProxy(
			func() (string, error) { return "", errDirect },
			nil,
			"",
		)
		if !errors.Is(err, errDirect) || usedProxy {
			t.Fatalf("got %q %v %v", val, usedProxy, err)
		}
	})

	t.Run("proxy fn not called when proxy empty", func(t *testing.T) {
		t.Parallel()
		called := false
		_, _, _ = TryDirectThenProxy(
			func() (string, error) { return "", errDirect },
			func() (string, error) { called = true; return "", nil },
			"",
		)
		if called {
			t.Fatal("proxyFn should not be called when proxyURL is empty")
		}
	})
}
