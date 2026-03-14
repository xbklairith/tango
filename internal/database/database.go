// Package database manages the PostgreSQL lifecycle, connection pooling, and schema migrations.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"runtime"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	_ "github.com/lib/pq"

	"github.com/xb/ari/internal/config"
)

// Open initializes the database connection. In dev mode with no DatabaseURL,
// it starts embedded PostgreSQL first. The returned cleanup function must be
// called on shutdown to release resources (stop embedded PG, close pool).
func Open(ctx context.Context, cfg *config.Config) (*sql.DB, func(), error) {
	var (
		dsn     string
		epg     *embeddedpostgres.EmbeddedPostgres
		cleanup = func() {} // no-op default
	)

	if cfg.UseEmbeddedPostgres() {
		slog.Info("starting embedded postgresql", "data_dir", cfg.DataDir)

		pgDataDir := filepath.Join(cfg.DataDir, "postgres")
		pgRuntimeDir := filepath.Join(cfg.DataDir, "pg-runtime")
		pgPort := uint32(cfg.EmbeddedPGPort)

		// Pin a specific PG version known to work on both amd64 and arm64 (Apple Silicon).
		pgVersion := embeddedpostgres.V16
		slog.Info("configuring embedded postgresql", "version", pgVersion, "port", pgPort)

		epg = embeddedpostgres.NewDatabase(
			embeddedpostgres.DefaultConfig().
				Version(pgVersion).
				DataPath(pgDataDir).
				RuntimePath(pgRuntimeDir).
				Port(pgPort).
				Logger(io.Discard),
		)

		if err := epg.Start(); err != nil {
			return nil, nil, fmt.Errorf("starting embedded postgres (version=%s, platform=%s/%s): %w\n\n"+
				"Troubleshooting:\n"+
				"  - Check your network connection (the PG binary is downloaded on first run)\n"+
				"  - If on an unsupported platform, set ARI_DATABASE_URL to use an external PostgreSQL instance\n"+
				"  - Clear cached downloads: rm -rf %s",
				pgVersion, runtime.GOOS, runtime.GOARCH, err, pgDataDir)
		}

		dsn = fmt.Sprintf(
			"host=localhost port=%d user=postgres password=postgres dbname=postgres sslmode=disable",
			pgPort,
		)

		cleanup = func() {
			slog.Info("stopping embedded postgresql")
			if err := epg.Stop(); err != nil {
				slog.Error("failed to stop embedded postgres", "error", err)
			}
		}

		slog.Info("embedded postgresql started", "port", pgPort)
	} else {
		dsn = cfg.DatabaseURL
		slog.Info("connecting to external postgresql")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("opening database connection: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	// Verify connectivity
	if err := db.PingContext(ctx); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("pinging database: %w", err)
	}

	slog.Info("database connection established")

	// Wrap cleanup to also close the pool
	origCleanup := cleanup
	cleanup = func() {
		slog.Info("closing database connection pool")
		if err := db.Close(); err != nil {
			slog.Error("failed to close database", "error", err)
		}
		origCleanup()
	}

	return db, cleanup, nil
}
