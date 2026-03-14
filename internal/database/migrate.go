package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/pressly/goose/v3"
)

// Migrate runs all pending goose migrations from the embedded SQL files.
func Migrate(ctx context.Context, db *sql.DB) error {
	slog.Info("running database migrations")

	goose.SetBaseFS(migrationsFS)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("setting goose dialect: %w", err)
	}

	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	version, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		return fmt.Errorf("getting migration version: %w", err)
	}

	slog.Info("migrations complete", "version", version)
	return nil
}
