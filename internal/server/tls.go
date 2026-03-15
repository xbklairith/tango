package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/xb/ari/internal/config"
)

// ResolveTLSConfig builds a *tls.Config based on the application configuration.
// Returns nil when TLS is not configured.
func ResolveTLSConfig(cfg *config.Config) (*tls.Config, error) {
	// Case 1: User-provided cert/key
	if cfg.TLSCert != "" && cfg.TLSKey != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
		if err != nil {
			return nil, fmt.Errorf("loading TLS cert/key: %w", err)
		}
		return &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}, nil
	}

	// Case 2: Domain set but no cert/key — would use autocert.
	// We skip autocert to avoid adding golang.org/x/crypto/acme/autocert dependency
	// if not already available. Users should provide cert+key for now.
	if cfg.TLSDomain != "" {
		slog.Warn("ARI_DOMAIN is set but no TLS cert/key provided; auto-TLS is not yet supported, running without TLS")
	}

	// Case 3: No TLS
	return nil, nil
}

// hstsMiddleware adds HSTS headers when TLS is enabled.
func hstsMiddleware(enabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if !enabled {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
			next.ServeHTTP(w, r)
		})
	}
}

// startRedirectServer starts an HTTP server that redirects all requests to HTTPS.
// It shuts down gracefully when the context is cancelled.
func startRedirectServer(ctx context.Context, port int, cfg ...*config.Config) {
	if port == 0 {
		port = 80
	}
	var expectedDomain string
	if len(cfg) > 0 && cfg[0] != nil {
		expectedDomain = cfg[0].TLSDomain
	}
	redirect := &http.Server{
		Addr: fmt.Sprintf(":%d", port),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := r.Host
			if expectedDomain != "" && host != expectedDomain {
				http.Error(w, "invalid host", http.StatusBadRequest)
				return
			}
			target := "https://" + host + r.URL.RequestURI()
			http.Redirect(w, r, target, http.StatusMovedPermanently)
		}),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("http redirect server listening", "addr", redirect.Addr)
		if err := redirect.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http redirect server error", "error", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := redirect.Shutdown(shutdownCtx); err != nil {
		slog.Error("http redirect server shutdown error", "error", err)
	}
}
