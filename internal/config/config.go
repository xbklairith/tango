// Package config loads and validates application configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// OAuthProviderConfig holds the credentials for a single OAuth2 provider.
type OAuthProviderConfig struct {
	ClientID     string
	ClientSecret string
}

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

	// EmbeddedPGPort is the port for embedded PostgreSQL (default 5433).
	EmbeddedPGPort int

	// DeploymentMode determines auth behavior: "local_trusted" or "authenticated".
	DeploymentMode string

	// JWTSecret is the HS256 signing key. Auto-generated if empty in authenticated mode.
	JWTSecret string

	// SessionTTL is the JWT and session expiration duration.
	SessionTTL time.Duration

	// DisableSignUp prevents new user registration (except first user).
	DisableSignUp bool

	// MaxRunsPerSquad is the max concurrent agent runs per squad (default 3).
	MaxRunsPerSquad int

	// StaleCheckoutAge is how long a checkout lock can be held before auto-release (default 2h).
	StaleCheckoutAge time.Duration

	// AgentDrainTimeout is the graceful shutdown timeout for running agents (default 30s).
	AgentDrainTimeout time.Duration

	// OAuthGoogle holds Google OAuth2 provider credentials.
	OAuthGoogle OAuthProviderConfig

	// OAuthGitHub holds GitHub OAuth2 provider credentials.
	OAuthGitHub OAuthProviderConfig

	// TLSCert is the path to the TLS certificate file.
	TLSCert string

	// TLSKey is the path to the TLS private key file.
	TLSKey string

	// TLSDomain is the domain for auto-TLS via Let's Encrypt.
	TLSDomain string

	// TLSRedirectPort is the HTTP port for TLS redirect (default 80).
	TLSRedirectPort int

	// RateLimitRPS is the per-IP requests per second limit (default 100).
	RateLimitRPS int

	// RateLimitBurst is the per-IP burst capacity (default 200).
	RateLimitBurst int

	// TrustedProxies is a comma-separated list of CIDR ranges for trusted proxy IPs.
	TrustedProxies string
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

// OAuthGoogleEnabled returns true when Google OAuth credentials are configured.
func (c *Config) OAuthGoogleEnabled() bool {
	return c.OAuthGoogle.ClientID != "" && c.OAuthGoogle.ClientSecret != ""
}

// OAuthGitHubEnabled returns true when GitHub OAuth credentials are configured.
func (c *Config) OAuthGitHubEnabled() bool {
	return c.OAuthGitHub.ClientID != "" && c.OAuthGitHub.ClientSecret != ""
}

// TLSEnabled returns true when TLS is configured (cert+key or domain).
func (c *Config) TLSEnabled() bool {
	return (c.TLSCert != "" && c.TLSKey != "") || c.TLSDomain != ""
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
		EmbeddedPGPort:  5433,
		DeploymentMode:  envOrDefault("ARI_DEPLOYMENT_MODE", "local_trusted"),
		JWTSecret:       os.Getenv("ARI_JWT_SECRET"),
		SessionTTL:      24 * time.Hour,
		DisableSignUp:    os.Getenv("ARI_DISABLE_SIGNUP") == "true",
		MaxRunsPerSquad:   3,
		StaleCheckoutAge:  2 * time.Hour,
		AgentDrainTimeout: 30 * time.Second,
		OAuthGoogle: OAuthProviderConfig{
			ClientID:     os.Getenv("ARI_OAUTH_GOOGLE_CLIENT_ID"),
			ClientSecret: os.Getenv("ARI_OAUTH_GOOGLE_CLIENT_SECRET"),
		},
		OAuthGitHub: OAuthProviderConfig{
			ClientID:     os.Getenv("ARI_OAUTH_GITHUB_CLIENT_ID"),
			ClientSecret: os.Getenv("ARI_OAUTH_GITHUB_CLIENT_SECRET"),
		},
		TLSCert:        os.Getenv("ARI_TLS_CERT"),
		TLSKey:         os.Getenv("ARI_TLS_KEY"),
		TLSDomain:      os.Getenv("ARI_DOMAIN"),
		TLSRedirectPort: 80,
		RateLimitRPS:    100,
		RateLimitBurst:  200,
		TrustedProxies:  os.Getenv("ARI_TRUSTED_PROXIES"),
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

	// Parse embedded PG port
	if v := os.Getenv("ARI_EMBEDDED_PG_PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid ARI_EMBEDDED_PG_PORT %q: %w", v, err)
		}
		if port < 1 || port > 65535 {
			return nil, fmt.Errorf("ARI_EMBEDDED_PG_PORT %d out of range (1-65535)", port)
		}
		cfg.EmbeddedPGPort = port
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

	// Parse session TTL
	if v := os.Getenv("ARI_SESSION_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid ARI_SESSION_TTL %q: %w", v, err)
		}
		cfg.SessionTTL = d
	}

	// Validate deployment mode
	switch cfg.DeploymentMode {
	case "local_trusted", "authenticated":
		// valid
	default:
		return nil, fmt.Errorf("invalid ARI_DEPLOYMENT_MODE %q: must be local_trusted or authenticated", cfg.DeploymentMode)
	}

	// Parse max runs per squad
	if v := os.Getenv("ARI_MAX_RUNS_PER_SQUAD"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("invalid ARI_MAX_RUNS_PER_SQUAD %q: must be a positive integer", v)
		}
		cfg.MaxRunsPerSquad = n
	}

	// Parse stale checkout age
	if v := os.Getenv("ARI_STALE_CHECKOUT_AGE"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid ARI_STALE_CHECKOUT_AGE %q: %w", v, err)
		}
		cfg.StaleCheckoutAge = d
	}

	// Parse agent drain timeout
	if v := os.Getenv("ARI_AGENT_DRAIN_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid ARI_AGENT_DRAIN_TIMEOUT %q: %w", v, err)
		}
		cfg.AgentDrainTimeout = d
	}

	// M1: In local_trusted mode, force bind to loopback for any non-loopback host
	if cfg.DeploymentMode == "local_trusted" && cfg.Host != "127.0.0.1" && cfg.Host != "localhost" && cfg.Host != "::1" {
		cfg.Host = "127.0.0.1"
	}

	// M7: Validate SessionTTL is positive
	if cfg.SessionTTL <= 0 {
		return nil, fmt.Errorf("ARI_SESSION_TTL must be positive, got %v", cfg.SessionTTL)
	}

	// Parse TLS redirect port
	if v := os.Getenv("ARI_TLS_REDIRECT_PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid ARI_TLS_REDIRECT_PORT %q: %w", v, err)
		}
		if port < 1 || port > 65535 {
			return nil, fmt.Errorf("ARI_TLS_REDIRECT_PORT %d out of range (1-65535)", port)
		}
		cfg.TLSRedirectPort = port
	}

	// Parse rate limit RPS
	if v := os.Getenv("ARI_RATE_LIMIT_RPS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid ARI_RATE_LIMIT_RPS %q: %w", v, err)
		}
		cfg.RateLimitRPS = n
	}

	// Parse rate limit burst
	if v := os.Getenv("ARI_RATE_LIMIT_BURST"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid ARI_RATE_LIMIT_BURST %q: %w", v, err)
		}
		cfg.RateLimitBurst = n
	}

	// Cross-field validation: TLS cert and key must both be set or both empty
	if (cfg.TLSCert != "") != (cfg.TLSKey != "") {
		return nil, fmt.Errorf("ARI_TLS_CERT and ARI_TLS_KEY must both be set or both empty")
	}

	// Cross-field validation: HTTP port and embedded PG port must not collide
	if cfg.UseEmbeddedPostgres() && cfg.Port == cfg.EmbeddedPGPort {
		return nil, fmt.Errorf("ARI_PORT and ARI_EMBEDDED_PG_PORT must not be the same (%d)", cfg.Port)
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
