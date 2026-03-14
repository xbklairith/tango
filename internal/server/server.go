// Package server provides the HTTP server, routing, and middleware for the Ari API.
package server

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/xb/ari/internal/config"
)

// Server wraps the HTTP server with application dependencies.
type Server struct {
	cfg     *config.Config
	db      *sql.DB
	version string
	http    *http.Server
}

// New creates a new Server with routes and middleware configured.
func New(cfg *config.Config, db *sql.DB, version string) *Server {
	s := &Server{
		cfg:     cfg,
		db:      db,
		version: version,
	}

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	handler := s.middleware(mux)

	s.http = &http.Server{
		Addr:         cfg.Addr(),
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

// ListenAndServe starts the HTTP server and blocks until the context is cancelled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		slog.Info("http server listening", "addr", s.http.Addr)
		if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("http server error: %w", err)
	case <-ctx.Done():
		slog.Info("shutting down http server", "timeout", s.cfg.ShutdownTimeout)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
	defer cancel()

	if err := s.http.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("http server shutdown: %w", err)
	}

	slog.Info("http server stopped")
	return nil
}
