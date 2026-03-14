package database

import (
	"context"
	"net"
	"testing"

	"github.com/xb/ari/internal/config"
)

func freePGPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func TestOpen_EmbeddedPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping embedded PG test")
	}

	ctx := context.Background()
	cfg := &config.Config{
		Env:            "development",
		DataDir:        t.TempDir(),
		EmbeddedPGPort: freePGPort(t),
	}

	db, cleanup, err := Open(ctx, cfg)
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	if db == nil {
		t.Fatal("Open() returned nil db")
	}
	if cleanup == nil {
		t.Fatal("Open() returned nil cleanup")
	}

	// Verify connectivity
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("PingContext() failed: %v", err)
	}

	// Cleanup and verify resources are released
	cleanup()

	if err := db.PingContext(ctx); err == nil {
		t.Error("PingContext() should fail after cleanup")
	}
}

func TestMigrate_AppliesPendingMigrations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping embedded PG test")
	}

	ctx := context.Background()
	cfg := &config.Config{
		Env:            "development",
		DataDir:        t.TempDir(),
		EmbeddedPGPort: freePGPort(t),
	}

	db, cleanup, err := Open(ctx, cfg)
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	defer cleanup()

	// Run migrations
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() returned error: %v", err)
	}

	// Verify migration was applied by checking goose version
	var version int64
	err = db.QueryRowContext(ctx, "SELECT version_id FROM goose_db_version ORDER BY id DESC LIMIT 1").Scan(&version)
	if err != nil {
		t.Fatalf("querying goose_db_version: %v", err)
	}
	if version == 0 {
		t.Error("migration version should be > 0")
	}
}
