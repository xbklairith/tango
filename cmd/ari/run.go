package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/xb/ari/internal/config"
	"github.com/xb/ari/internal/database"
	"github.com/xb/ari/internal/server"
)

func newRunCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Start the Ari server",
		Long:  "Start the full Ari stack: database, migrations, and HTTP server.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServer(cmd.Context(), version)
		},
	}

	cmd.Flags().Int("port", 0, "HTTP server port (overrides ARI_PORT)")

	return cmd
}

func runServer(ctx context.Context, version string) error {
	// 1. Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
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

	// 5. Start HTTP server
	srv := server.New(cfg, db, version)

	// 6. Wait for shutdown signal
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	return srv.ListenAndServe(ctx)
}
