package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/xb/ari/internal/auth"
	"github.com/xb/ari/internal/config"
	"github.com/xb/ari/internal/database"
	dbpkg "github.com/xb/ari/internal/database/db"
	"github.com/xb/ari/internal/server"
	"github.com/xb/ari/internal/server/handlers"
)

func newRunCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Start the Ari server",
		Long:  "Start the full Ari stack: database, migrations, and HTTP server.",
		RunE: func(cmd *cobra.Command, args []string) error {
			port, _ := cmd.Flags().GetInt("port")
			return runServer(cmd.Context(), version, port)
		},
	}

	cmd.Flags().Int("port", 0, "HTTP server port (overrides ARI_PORT)")

	return cmd
}

func runServer(ctx context.Context, version string, portOverride int) error {
	// 1. Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Apply --port flag override
	if portOverride > 0 {
		cfg.Port = portOverride
	}

	// 2. Initialize logger
	setupLogger(cfg.LogLevel, cfg.Env)

	slog.Info("starting ari", "version", version, "env", cfg.Env)

	// 3. Start database (embedded or external)
	db, cleanup, err := database.Open(ctx, cfg)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer cleanup()

	// 4. Run migrations
	if err := database.Migrate(ctx, db); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	// 5. Initialize auth components
	queries := dbpkg.New(db)
	mode := auth.DeploymentMode(cfg.DeploymentMode)

	var jwtSvc *auth.JWTService
	var sessionStore auth.SessionStore
	rateLimiter := auth.NewRateLimiter(10, time.Minute)

	if mode == auth.ModeAuthenticated {
		// Resolve JWT signing key
		signingKey, err := resolveJWTKey(cfg)
		if err != nil {
			return fmt.Errorf("resolving JWT key: %w", err)
		}

		jwtSvc, err = auth.NewJWTService(signingKey, cfg.SessionTTL)
		if err != nil {
			return fmt.Errorf("creating JWT service: %w", err)
		}

		sessionStore = auth.NewPgSessionStore(queries)

		slog.Info("auth initialized", "mode", mode)
	} else {
		// M11: Warn when running in local_trusted mode
		slog.Warn("running in local_trusted mode — authentication is disabled, bind restricted to loopback")
	}

	// Start rate limiter cleanup
	done := make(chan struct{})
	rateLimiter.StartCleanup(done, 5*time.Minute)

	isSecure := cfg.Host != "127.0.0.1" && cfg.Host != "localhost"

	authHandler := handlers.NewAuthHandler(
		queries, db, jwtSvc, sessionStore, rateLimiter,
		mode, cfg.DisableSignUp, isSecure, cfg.SessionTTL,
	)

	squadHandler := handlers.NewSquadHandler(queries, db)
	membershipHandler := handlers.NewMembershipHandler(queries)
	agentHandler := handlers.NewAgentHandler(queries, db)

	// 6. Start HTTP server
	srv := server.New(cfg, db, version, mode, jwtSvc, sessionStore, authHandler, squadHandler, membershipHandler, agentHandler)

	// 7. Wait for shutdown signal
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// H5: Start session cleanup AFTER signal.NotifyContext so it uses the right context
	if sessionStore != nil {
		go auth.StartSessionCleanup(ctx, sessionStore, time.Hour)
	}

	err = srv.ListenAndServe(ctx)
	close(done)
	return err
}

// resolveJWTKey loads the JWT signing key from config or generates one.
func resolveJWTKey(cfg *config.Config) ([]byte, error) {
	if cfg.JWTSecret != "" {
		key, err := hex.DecodeString(cfg.JWTSecret)
		if err != nil {
			// Treat as raw bytes if not hex
			return []byte(cfg.JWTSecret), nil
		}
		return key, nil
	}

	// Auto-generate and persist
	secretsDir := filepath.Join(cfg.DataDir, "secrets")
	keyPath := filepath.Join(secretsDir, "jwt.key")

	// Try to load existing key
	if data, err := os.ReadFile(keyPath); err == nil && len(data) >= 32 {
		slog.Info("loaded JWT key from disk", "path", keyPath)
		return data, nil
	}

	// Generate new key
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generating JWT key: %w", err)
	}

	if err := os.MkdirAll(secretsDir, 0700); err != nil {
		return nil, fmt.Errorf("creating secrets directory: %w", err)
	}
	if err := os.WriteFile(keyPath, key, 0600); err != nil {
		return nil, fmt.Errorf("writing JWT key: %w", err)
	}

	slog.Info("generated and persisted JWT key", "path", keyPath)
	return key, nil
}
