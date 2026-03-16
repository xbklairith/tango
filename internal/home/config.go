package home

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// FileConfig represents the JSON config file schema.
type FileConfig struct {
	Meta     *ConfigMeta     `json:"$meta,omitempty"`
	Database *DatabaseConfig `json:"database,omitempty"`
	Server   *ServerConfig   `json:"server,omitempty"`
	Auth     *AuthConfig     `json:"auth,omitempty"`
	Logging  *LoggingConfig  `json:"logging,omitempty"`
	Secrets  *SecretsConfig  `json:"secrets,omitempty"`
	Storage  *StorageConfig  `json:"storage,omitempty"`
}

type ConfigMeta struct {
	Version   int    `json:"version"`
	UpdatedAt string `json:"updatedAt,omitempty"`
	Source    string `json:"source,omitempty"`
}

type DatabaseConfig struct {
	Mode                    string        `json:"mode"`
	ConnectionString        string        `json:"connectionString,omitempty"`
	EmbeddedPostgresDataDir string        `json:"embeddedPostgresDataDir,omitempty"`
	EmbeddedPostgresPort    int           `json:"embeddedPostgresPort,omitempty"`
	Backup                  *BackupConfig `json:"backup,omitempty"`
}

type BackupConfig struct {
	Enabled         bool   `json:"enabled"`
	IntervalMinutes int    `json:"intervalMinutes"`
	RetentionDays   int    `json:"retentionDays"`
	Dir             string `json:"dir,omitempty"`
}

type ServerConfig struct {
	DeploymentMode         string `json:"deploymentMode,omitempty"`
	Host                   string `json:"host,omitempty"`
	Port                   int    `json:"port,omitempty"`
	ServeUI                bool   `json:"serveUi,omitempty"`
	ShutdownTimeoutSeconds int    `json:"shutdownTimeoutSeconds,omitempty"`
	RateLimitRPS           int    `json:"rateLimitRPS,omitempty"`
	RateLimitBurst         int    `json:"rateLimitBurst,omitempty"`
	TrustedProxies         string `json:"trustedProxies,omitempty"`
}

type AuthConfig struct {
	BaseURLMode       string `json:"baseUrlMode,omitempty"`
	DisableSignUp     bool   `json:"disableSignUp,omitempty"`
	SessionTTLMinutes int    `json:"sessionTTLMinutes,omitempty"`
	JWTSecret         string `json:"jwtSecret,omitempty"`
}

type LoggingConfig struct {
	Level  string `json:"level,omitempty"`
	Mode   string `json:"mode,omitempty"`
	LogDir string `json:"logDir,omitempty"`
}

type SecretsConfig struct {
	Provider    string `json:"provider,omitempty"`
	KeyFilePath string `json:"keyFilePath,omitempty"`
}

type StorageConfig struct {
	Provider  string           `json:"provider,omitempty"`
	LocalDisk *LocalDiskConfig `json:"localDisk,omitempty"`
}

type LocalDiskConfig struct {
	BaseDir string `json:"baseDir"`
}

// DefaultConfig returns a FileConfig with all sensible defaults.
func DefaultConfig() *FileConfig {
	return &FileConfig{
		Meta: &ConfigMeta{
			Version:   1,
			UpdatedAt: time.Now().UTC().Format(time.RFC3339),
			Source:    "default",
		},
		Database: &DatabaseConfig{
			Mode:                 "embedded-postgres",
			EmbeddedPostgresPort: 5433,
			Backup: &BackupConfig{
				Enabled:         true,
				IntervalMinutes: 60,
				RetentionDays:   30,
			},
		},
		Server: &ServerConfig{
			DeploymentMode:         "local_trusted",
			Host:                   "0.0.0.0",
			Port:                   3100,
			ServeUI:                true,
			ShutdownTimeoutSeconds: 30,
			RateLimitRPS:           100,
			RateLimitBurst:         200,
		},
		Auth: &AuthConfig{
			BaseURLMode:       "auto",
			SessionTTLMinutes: 1440,
		},
		Logging: &LoggingConfig{
			Level: "info",
			Mode:  "stderr",
		},
		Secrets: &SecretsConfig{
			Provider: "local_encrypted",
		},
		Storage: &StorageConfig{
			Provider:  "local_disk",
			LocalDisk: &LocalDiskConfig{},
		},
	}
}

// LoadConfigFile reads and parses a JSON config file.
// Returns nil, nil if the file does not exist.
func LoadConfigFile(path string) (*FileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg FileConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}

	return &cfg, nil
}

// WriteConfigFile writes a FileConfig to disk as formatted JSON.
func WriteConfigFile(path string, cfg *FileConfig) error {
	if cfg.Meta != nil {
		cfg.Meta.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

// MergeConfigs merges overlay values onto base. Non-zero overlay values take precedence.
func MergeConfigs(base, overlay *FileConfig) *FileConfig {
	if overlay == nil {
		return base
	}

	result := *base

	if overlay.Database != nil {
		merged := mergeDatabase(base.Database, overlay.Database)
		result.Database = merged
	}
	if overlay.Server != nil {
		merged := mergeServer(base.Server, overlay.Server)
		result.Server = merged
	}
	if overlay.Auth != nil {
		merged := mergeAuth(base.Auth, overlay.Auth)
		result.Auth = merged
	}
	if overlay.Logging != nil {
		merged := mergeLogging(base.Logging, overlay.Logging)
		result.Logging = merged
	}
	if overlay.Secrets != nil {
		merged := mergeSecrets(base.Secrets, overlay.Secrets)
		result.Secrets = merged
	}
	if overlay.Storage != nil {
		merged := mergeStorage(base.Storage, overlay.Storage)
		result.Storage = merged
	}

	return &result
}

// DiscoverConfigPath finds the config file using the search order:
// 1. overridePath (explicit --config flag or ARI_CONFIG env)
// 2. Walk up from startDir looking for .ari/config.json
// Returns empty string if not found.
func DiscoverConfigPath(overridePath string, startDir string) string {
	if overridePath != "" {
		return filepath.Clean(overridePath)
	}
	if v := os.Getenv("ARI_CONFIG"); v != "" {
		return expandHomePrefix(v)
	}
	return findConfigFromAncestors(startDir)
}

func findConfigFromAncestors(startDir string) string {
	dir := filepath.Clean(startDir)
	maxDepth := 10
	for range maxDepth {
		candidate := filepath.Join(dir, ".ari", ConfigFileName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func mergeDatabase(base, overlay *DatabaseConfig) *DatabaseConfig {
	if base == nil {
		return overlay
	}
	result := *base
	if overlay.Mode != "" {
		result.Mode = overlay.Mode
	}
	if overlay.ConnectionString != "" {
		result.ConnectionString = overlay.ConnectionString
	}
	if overlay.EmbeddedPostgresDataDir != "" {
		result.EmbeddedPostgresDataDir = overlay.EmbeddedPostgresDataDir
	}
	if overlay.EmbeddedPostgresPort != 0 {
		result.EmbeddedPostgresPort = overlay.EmbeddedPostgresPort
	}
	if overlay.Backup != nil {
		result.Backup = overlay.Backup
	}
	return &result
}

func mergeServer(base, overlay *ServerConfig) *ServerConfig {
	if base == nil {
		return overlay
	}
	result := *base
	if overlay.DeploymentMode != "" {
		result.DeploymentMode = overlay.DeploymentMode
	}
	if overlay.Host != "" {
		result.Host = overlay.Host
	}
	if overlay.Port != 0 {
		result.Port = overlay.Port
	}
	if overlay.ShutdownTimeoutSeconds != 0 {
		result.ShutdownTimeoutSeconds = overlay.ShutdownTimeoutSeconds
	}
	if overlay.RateLimitRPS != 0 {
		result.RateLimitRPS = overlay.RateLimitRPS
	}
	if overlay.RateLimitBurst != 0 {
		result.RateLimitBurst = overlay.RateLimitBurst
	}
	if overlay.TrustedProxies != "" {
		result.TrustedProxies = overlay.TrustedProxies
	}
	return &result
}

func mergeAuth(base, overlay *AuthConfig) *AuthConfig {
	if base == nil {
		return overlay
	}
	result := *base
	if overlay.BaseURLMode != "" {
		result.BaseURLMode = overlay.BaseURLMode
	}
	if overlay.SessionTTLMinutes != 0 {
		result.SessionTTLMinutes = overlay.SessionTTLMinutes
	}
	if overlay.JWTSecret != "" {
		result.JWTSecret = overlay.JWTSecret
	}
	// DisableSignUp is a bool — overlay always wins if Auth section is present
	result.DisableSignUp = overlay.DisableSignUp
	return &result
}

func mergeLogging(base, overlay *LoggingConfig) *LoggingConfig {
	if base == nil {
		return overlay
	}
	result := *base
	if overlay.Level != "" {
		result.Level = overlay.Level
	}
	if overlay.Mode != "" {
		result.Mode = overlay.Mode
	}
	if overlay.LogDir != "" {
		result.LogDir = overlay.LogDir
	}
	return &result
}

func mergeSecrets(base, overlay *SecretsConfig) *SecretsConfig {
	if base == nil {
		return overlay
	}
	result := *base
	if overlay.Provider != "" {
		result.Provider = overlay.Provider
	}
	if overlay.KeyFilePath != "" {
		result.KeyFilePath = overlay.KeyFilePath
	}
	return &result
}

func mergeStorage(base, overlay *StorageConfig) *StorageConfig {
	if base == nil {
		return overlay
	}
	result := *base
	if overlay.Provider != "" {
		result.Provider = overlay.Provider
	}
	if overlay.LocalDisk != nil {
		result.LocalDisk = overlay.LocalDisk
	}
	return &result
}
