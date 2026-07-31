// Package server runs the simplearchive HTTP server (chi). It owns the router,
// the http.Server lifecycle, and graceful shutdown on SIGINT/SIGTERM.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/nguyenmp/simplearchive/internal/meta"
)

const (
	defaultAddr       = "127.0.0.1:8080"
	readHeaderTimeout  = 10 * time.Second
	shutdownTimeout    = 10 * time.Second
)

// DefaultAddr returns the address the server binds to when SERVE_ADDR is unset.
func DefaultAddr() string { return defaultAddr }

// Server holds the dependencies and configuration for the HTTP server.
type Server struct {
	Logger      *slog.Logger
	DB          *meta.DB
	ArchiveRoot string
	Addr        string
	// Listener, when non-nil, is used instead of opening Addr. Tests inject a
	// net.Listener so they can dial the server without racing on a port.
	Listener net.Listener
}

// Router builds and returns the chi router wired with all routes. It is
// exported so tests (and future tooling) can mount the same routes behind an
// httptest server without starting the full http.Server.
func (s *Server) Router() http.Handler {
	if s.Logger == nil {
		s.Logger = slog.Default()
	}
	if s.ArchiveRoot == "" {
		s.ArchiveRoot = "archive"
	}

	r := chi.NewRouter()
	r.Get("/healthz", s.handleHealthz)
	r.Handle("/static/*", staticHandler())
	return r
}

// Run starts the HTTP server and blocks until the context is cancelled or a
// termination signal is received. It performs a graceful shutdown.
func (s *Server) Run(ctx context.Context) error {
	if s.Logger == nil {
		s.Logger = slog.Default()
	}
	if s.DB == nil {
		return errors.New("server: database not configured")
	}
	if s.Addr == "" {
		s.Addr = defaultAddr
	}

	ln := s.Listener
	if ln == nil {
		var err error
		ln, err = net.Listen("tcp", s.Addr)
		if err != nil {
			return fmt.Errorf("server: listen %q: %w", s.Addr, err)
		}
	}

	srv := &http.Server{
		Handler:           s.Router(),
		ReadHeaderTimeout: readHeaderTimeout,
	}
	// Track the listener's address in case it was auto-assigned (:0).
	s.Addr = ln.Addr().String()

	errCh := make(chan error, 1)
	go func() {
		s.Logger.Info("http server listening", "addr", s.Addr)
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	// Wait for context cancellation or a signal, whichever comes first.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("server: serve: %w", err)
		}
		return nil
	}

	s.Logger.Info("shutting down http server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server: shutdown: %w", err)
	}
	return nil
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}
