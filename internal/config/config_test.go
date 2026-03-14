package config

import (
	"os"
	"testing"
	"time"
)

// clearEnv unsets all ARI_* environment variables.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"ARI_ENV", "ARI_HOST", "ARI_PORT", "ARI_DATABASE_URL",
		"ARI_DATA_DIR", "ARI_LOG_LEVEL", "ARI_SHUTDOWN_TIMEOUT",
	} {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
}

func TestLoad_Defaults(t *testing.T) {
	clearEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.Port != 3100 {
		t.Errorf("Port = %d, want 3100", cfg.Port)
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("Host = %q, want %q", cfg.Host, "0.0.0.0")
	}
	if cfg.Env != "development" {
		t.Errorf("Env = %q, want %q", cfg.Env, "development")
	}
	if cfg.DatabaseURL != "" {
		t.Errorf("DatabaseURL = %q, want empty", cfg.DatabaseURL)
	}
	if cfg.DataDir != "./data" {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, "./data")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if cfg.ShutdownTimeout != 30*time.Second {
		t.Errorf("ShutdownTimeout = %v, want %v", cfg.ShutdownTimeout, 30*time.Second)
	}
	if cfg.UseEmbeddedPostgres() != true {
		t.Error("UseEmbeddedPostgres() = false, want true")
	}
	if cfg.IsProduction() != false {
		t.Error("IsProduction() = true, want false")
	}
	if cfg.Addr() != "0.0.0.0:3100" {
		t.Errorf("Addr() = %q, want %q", cfg.Addr(), "0.0.0.0:3100")
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	clearEnv(t)

	t.Setenv("ARI_PORT", "8080")
	t.Setenv("ARI_HOST", "127.0.0.1")
	t.Setenv("ARI_ENV", "production")
	t.Setenv("ARI_DATABASE_URL", "postgres://user:pass@host:5432/db")
	t.Setenv("ARI_DATA_DIR", "/tmp/ari")
	t.Setenv("ARI_LOG_LEVEL", "debug")
	t.Setenv("ARI_SHUTDOWN_TIMEOUT", "10s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.Host != "127.0.0.1" {
		t.Errorf("Host = %q, want %q", cfg.Host, "127.0.0.1")
	}
	if cfg.Env != "production" {
		t.Errorf("Env = %q, want %q", cfg.Env, "production")
	}
	if cfg.DatabaseURL != "postgres://user:pass@host:5432/db" {
		t.Errorf("DatabaseURL = %q, want postgres URL", cfg.DatabaseURL)
	}
	if cfg.DataDir != "/tmp/ari" {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, "/tmp/ari")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
	if cfg.ShutdownTimeout != 10*time.Second {
		t.Errorf("ShutdownTimeout = %v, want %v", cfg.ShutdownTimeout, 10*time.Second)
	}
	if cfg.UseEmbeddedPostgres() != false {
		t.Error("UseEmbeddedPostgres() = true, want false")
	}
	if cfg.IsProduction() != true {
		t.Error("IsProduction() = false, want true")
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	clearEnv(t)
	t.Setenv("ARI_PORT", "not-a-number")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() should return error for invalid port")
	}
}

func TestLoad_PortOutOfRange(t *testing.T) {
	clearEnv(t)
	t.Setenv("ARI_PORT", "99999")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() should return error for out-of-range port")
	}
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	clearEnv(t)
	t.Setenv("ARI_LOG_LEVEL", "verbose")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() should return error for invalid log level")
	}
}

func TestLoad_InvalidEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("ARI_ENV", "staging")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() should return error for invalid env")
	}
}

func TestLoad_InvalidShutdownTimeout(t *testing.T) {
	clearEnv(t)
	t.Setenv("ARI_SHUTDOWN_TIMEOUT", "notaduration")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() should return error for invalid shutdown timeout")
	}
}

func TestLoad_ExternalDatabase(t *testing.T) {
	clearEnv(t)
	t.Setenv("ARI_DATABASE_URL", "postgres://localhost/test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.UseEmbeddedPostgres() != false {
		t.Error("UseEmbeddedPostgres() = true, want false when DATABASE_URL is set")
	}
}
