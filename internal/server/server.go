// Package server provides the HTTP server, routing, and middleware for the Ari API.
package server

import (
	"context"
	"crypto/tls"
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
	cfg       *config.Config
	db        *sql.DB
	version   string
	webFS     fs.FS
	http      *http.Server
	tlsConfig *tls.Config
}

// RouteRegistrar can register additional routes on the mux.
type RouteRegistrar interface {
	RegisterRoutes(mux *http.ServeMux)
}

// ServerOptions holds optional configuration for the server.
type ServerOptions struct {
	TLSConfig   *tls.Config
	RateLimiter *RateLimitMiddleware
}

// New creates a new Server with routes and middleware configured.
func New(cfg *config.Config, db *sql.DB, version string, mode auth.DeploymentMode, jwtSvc *auth.JWTService, sessions auth.SessionStore, runTokenSvc *auth.RunTokenService, webFS fs.FS, opts *ServerOptions, extra ...RouteRegistrar) *Server {
	s := &Server{
		cfg:     cfg,
		db:      db,
		version: version,
		webFS:   webFS,
	}

	if opts != nil {
		s.tlsConfig = opts.TLSConfig
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
		handler = auth.Middleware(mode, jwtSvc, sessions, runTokenSvc)(handler)
	} else if mode == auth.ModeLocalTrusted {
		handler = auth.Middleware(mode, nil, nil, runTokenSvc)(handler)
	}

	// Limit request body size to 1MB before auth to prevent abuse
	handler = maxBodySize(1 << 20)(handler)

	// Per-IP rate limiting (before body size so rejected requests don't consume resources)
	if opts != nil && opts.RateLimiter != nil {
		handler = opts.RateLimiter.Middleware()(handler)
	}

	// HSTS middleware when TLS is active
	handler = hstsMiddleware(s.tlsConfig != nil)(handler)

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

// maxBodySize returns middleware that limits the size of incoming request bodies.
func maxBodySize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ListenAndServe starts the HTTP server and blocks until the context is cancelled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	errCh := make(chan error, 1)

	if s.tlsConfig != nil {
		s.http.TLSConfig = s.tlsConfig
		// Start HTTP-to-HTTPS redirect server
		go startRedirectServer(ctx, s.cfg.TLSRedirectPort)

		go func() {
			slog.Info("https server listening", "addr", s.http.Addr)
			if err := s.http.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				errCh <- err
			}
			close(errCh)
		}()
	} else {
		go func() {
			slog.Info("http server listening", "addr", s.http.Addr)
			if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- err
			}
			close(errCh)
		}()
	}

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
