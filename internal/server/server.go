// Package server provides the HTTP server, routing, and middleware for the Ari API.
package server

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/xb/ari/internal/auth"
	"github.com/xb/ari/internal/config"
)

// Server wraps the HTTP server with application dependencies.
type Server struct {
	cfg     *config.Config
	db      *sql.DB
	version string
	webFS   fs.FS
	http    *http.Server
}

// RouteRegistrar can register additional routes on the mux.
type RouteRegistrar interface {
	RegisterRoutes(mux *http.ServeMux)
}

// New creates a new Server with routes and middleware configured.
func New(cfg *config.Config, db *sql.DB, version string, mode auth.DeploymentMode, jwtSvc *auth.JWTService, sessions auth.SessionStore, webFS fs.FS, extra ...RouteRegistrar) *Server {
	s := &Server{
		cfg:     cfg,
		db:      db,
		version: version,
		webFS:   webFS,
	}

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	// Register additional route handlers (e.g., auth)
	for _, r := range extra {
		if r != nil {
			r.RegisterRoutes(mux)
		}
	}

	// SPA catch-all for non-API routes (serves embedded frontend)
	if webFS != nil {
		mux.Handle("/", spaHandler(webFS))
	}

	// Apply auth middleware before the main middleware chain
	var handler http.Handler = mux
	if mode == auth.ModeAuthenticated && jwtSvc != nil && sessions != nil {
		handler = auth.Middleware(mode, jwtSvc, sessions)(handler)
	} else if mode == auth.ModeLocalTrusted {
		handler = auth.Middleware(mode, nil, nil)(handler)
	}

	handler = s.middleware(handler)

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
