// Package config loads and validates application configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all application configuration values.
type Config struct {
	// Env is the runtime environment: "development" or "production".
	Env string

	// Host is the HTTP server bind address.
	Host string

	// Port is the HTTP server listen port.
	Port int

	// DatabaseURL is the PostgreSQL connection string.
	// When empty, embedded PostgreSQL is used (dev mode).
	DatabaseURL string

	// DataDir is the directory for embedded PG data and local storage.
	DataDir string

	// LogLevel controls logging verbosity: "debug", "info", "warn", "error".
	LogLevel string

	// ShutdownTimeout is the graceful shutdown deadline.
	ShutdownTimeout time.Duration
}

// IsProduction returns true when running in production mode.
func (c *Config) IsProduction() bool {
	return c.Env == "production"
}

// UseEmbeddedPostgres returns true when no external database URL is configured.
func (c *Config) UseEmbeddedPostgres() bool {
	return c.DatabaseURL == ""
}

// Addr returns the "host:port" listen address.
func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// Load reads configuration from environment variables with sensible defaults.
func Load() (*Config, error) {
	cfg := &Config{
		Env:             envOrDefault("ARI_ENV", "development"),
		Host:            envOrDefault("ARI_HOST", "0.0.0.0"),
		Port:            3100,
		DatabaseURL:     os.Getenv("ARI_DATABASE_URL"),
		DataDir:         envOrDefault("ARI_DATA_DIR", "./data"),
		LogLevel:        envOrDefault("ARI_LOG_LEVEL", "info"),
		ShutdownTimeout: 30 * time.Second,
	}

	// Parse port
	if v := os.Getenv("ARI_PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid ARI_PORT %q: %w", v, err)
		}
		if port < 1 || port > 65535 {
			return nil, fmt.Errorf("ARI_PORT %d out of range (1-65535)", port)
		}
		cfg.Port = port
	}

	// Parse shutdown timeout
	if v := os.Getenv("ARI_SHUTDOWN_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid ARI_SHUTDOWN_TIMEOUT %q: %w", v, err)
		}
		cfg.ShutdownTimeout = d
	}

	// Validate log level
	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
		// valid
	default:
		return nil, fmt.Errorf("invalid ARI_LOG_LEVEL %q: must be debug, info, warn, or error", cfg.LogLevel)
	}

	// Validate env
	switch cfg.Env {
	case "development", "production":
		// valid
	default:
		return nil, fmt.Errorf("invalid ARI_ENV %q: must be development or production", cfg.Env)
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
