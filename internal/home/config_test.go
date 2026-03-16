package home

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}
	if cfg.Server == nil || cfg.Server.Port != 3100 {
		t.Errorf("default server port = %v, want 3100", cfg.Server)
	}
	if cfg.Logging == nil || cfg.Logging.Level != "info" {
		t.Errorf("default log level = %v, want info", cfg.Logging)
	}
	if cfg.Database == nil || cfg.Database.Mode != "embedded-postgres" {
		t.Errorf("default database mode = %v, want embedded-postgres", cfg.Database)
	}
	if cfg.Database.EmbeddedPostgresPort != 5433 {
		t.Errorf("default embedded PG port = %d, want 5433", cfg.Database.EmbeddedPostgresPort)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("default host = %q, want 0.0.0.0", cfg.Server.Host)
	}
	if cfg.Server.DeploymentMode != "local_trusted" {
		t.Errorf("default deployment mode = %q, want local_trusted", cfg.Server.DeploymentMode)
	}
}

func TestLoadConfigFromFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	data := `{
		"$meta": {"version": 1},
		"database": {"mode": "postgres", "connectionString": "postgres://localhost/test", "embeddedPostgresPort": 5555},
		"server": {"host": "127.0.0.1", "port": 9000, "deploymentMode": "authenticated"},
		"logging": {"level": "debug"},
		"secrets": {"keyFilePath": "/tmp/key"}
	}`
	if err := os.WriteFile(cfgPath, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfigFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfigFile() error: %v", err)
	}

	if cfg.Database.Mode != "postgres" {
		t.Errorf("Database.Mode = %q, want postgres", cfg.Database.Mode)
	}
	if cfg.Database.ConnectionString != "postgres://localhost/test" {
		t.Errorf("Database.ConnectionString = %q", cfg.Database.ConnectionString)
	}
	if cfg.Database.EmbeddedPostgresPort != 5555 {
		t.Errorf("Database.EmbeddedPostgresPort = %d, want 5555", cfg.Database.EmbeddedPostgresPort)
	}
	if cfg.Server.Port != 9000 {
		t.Errorf("Server.Port = %d, want 9000", cfg.Server.Port)
	}
	if cfg.Server.DeploymentMode != "authenticated" {
		t.Errorf("Server.DeploymentMode = %q, want authenticated", cfg.Server.DeploymentMode)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Logging.Level = %q, want debug", cfg.Logging.Level)
	}
	if cfg.Secrets.KeyFilePath != "/tmp/key" {
		t.Errorf("Secrets.KeyFilePath = %q", cfg.Secrets.KeyFilePath)
	}
}

func TestLoadConfigFromFile_PartialOverride(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	data := `{"server": {"port": 8080}}`
	if err := os.WriteFile(cfgPath, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfigFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfigFile() error: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want 8080", cfg.Server.Port)
	}
	// Other fields should be zero/nil (not merged with defaults in LoadConfigFile)
}

func TestLoadConfigFromFile_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	if err := os.WriteFile(cfgPath, []byte(`{invalid`), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfigFile(cfgPath)
	if err == nil {
		t.Fatal("LoadConfigFile() should return error for invalid JSON")
	}
}

func TestLoadConfigFromFile_NotFound(t *testing.T) {
	cfg, err := LoadConfigFile("/nonexistent/config.json")
	if err != nil {
		t.Fatalf("LoadConfigFile() should not error for missing file, got: %v", err)
	}
	if cfg != nil {
		t.Errorf("LoadConfigFile() should return nil for missing file")
	}
}

func TestMergeConfigs(t *testing.T) {
	base := DefaultConfig()
	overlay := &FileConfig{
		Server: &ServerConfig{Port: 9000},
	}

	result := MergeConfigs(base, overlay)
	if result.Server.Port != 9000 {
		t.Errorf("merged port = %d, want 9000 (overlay)", result.Server.Port)
	}
	// Non-overlaid fields should retain base defaults
	if result.Server.Host != "127.0.0.1" {
		t.Errorf("merged host = %q, want 0.0.0.0 (base)", result.Server.Host)
	}
	if result.Database.Mode != "embedded-postgres" {
		t.Errorf("merged database mode = %q, want embedded-postgres (base)", result.Database.Mode)
	}
}

func TestDiscoverConfig_InCwd(t *testing.T) {
	dir := t.TempDir()
	ariDir := filepath.Join(dir, ".ari")
	os.MkdirAll(ariDir, 0755)
	os.WriteFile(filepath.Join(ariDir, "config.json"), []byte(`{}`), 0644)

	found := DiscoverConfigPath("", dir)
	want := filepath.Join(ariDir, "config.json")
	if found != want {
		t.Errorf("DiscoverConfigPath() = %q, want %q", found, want)
	}
}

func TestDiscoverConfig_InParent(t *testing.T) {
	parent := t.TempDir()
	ariDir := filepath.Join(parent, ".ari")
	os.MkdirAll(ariDir, 0755)
	os.WriteFile(filepath.Join(ariDir, "config.json"), []byte(`{}`), 0644)

	child := filepath.Join(parent, "subdir", "deep")
	os.MkdirAll(child, 0755)

	found := DiscoverConfigPath("", child)
	want := filepath.Join(ariDir, "config.json")
	if found != want {
		t.Errorf("DiscoverConfigPath() = %q, want %q", found, want)
	}
}

func TestDiscoverConfig_NotFound(t *testing.T) {
	dir := t.TempDir()

	found := DiscoverConfigPath("", dir)
	if found != "" {
		t.Errorf("DiscoverConfigPath() = %q, want empty", found)
	}
}

func TestDiscoverConfig_ExplicitPath(t *testing.T) {
	found := DiscoverConfigPath("/explicit/config.json", "/some/dir")
	if found != "/explicit/config.json" {
		t.Errorf("DiscoverConfigPath() = %q, want /explicit/config.json", found)
	}
}

func TestWriteConfigFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfg := DefaultConfig()
	if err := WriteConfigFile(cfgPath, cfg); err != nil {
		t.Fatalf("WriteConfigFile() error: %v", err)
	}

	// Read back and verify it's valid JSON
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	var parsed FileConfig
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("written config is not valid JSON: %v", err)
	}

	if parsed.Server.Port != 3100 {
		t.Errorf("written port = %d, want 3100", parsed.Server.Port)
	}
	if parsed.Meta == nil || parsed.Meta.Version != 1 {
		t.Errorf("written meta version = %v, want 1", parsed.Meta)
	}
}

func TestConfigSections_RoundTrip(t *testing.T) {
	cfg := &FileConfig{
		Meta: &ConfigMeta{Version: 1, Source: "test"},
		Database: &DatabaseConfig{
			Mode:                    "embedded-postgres",
			EmbeddedPostgresPort:    5433,
			Backup: &BackupConfig{
				Enabled:         true,
				IntervalMinutes: 60,
				RetentionDays:   30,
			},
		},
		Server:  &ServerConfig{Host: "127.0.0.1", Port: 3100, DeploymentMode: "local_trusted"},
		Auth:    &AuthConfig{DisableSignUp: false, SessionTTLMinutes: 1440},
		Logging: &LoggingConfig{Level: "info", Mode: "stderr"},
		Secrets: &SecretsConfig{Provider: "local_encrypted", KeyFilePath: "/tmp/key"},
		Storage: &StorageConfig{Provider: "local_disk", LocalDisk: &LocalDiskConfig{BaseDir: "/tmp/storage"}},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed FileConfig
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if parsed.Database.Mode != "embedded-postgres" {
		t.Errorf("Database.Mode = %q", parsed.Database.Mode)
	}
	if parsed.Database.Backup.IntervalMinutes != 60 {
		t.Errorf("Database.Backup.IntervalMinutes = %d", parsed.Database.Backup.IntervalMinutes)
	}
	if parsed.Server.Port != 3100 {
		t.Errorf("Server.Port = %d", parsed.Server.Port)
	}
	if parsed.Auth.SessionTTLMinutes != 1440 {
		t.Errorf("Auth.SessionTTLMinutes = %d", parsed.Auth.SessionTTLMinutes)
	}
	if parsed.Storage.LocalDisk.BaseDir != "/tmp/storage" {
		t.Errorf("Storage.LocalDisk.BaseDir = %q", parsed.Storage.LocalDisk.BaseDir)
	}
}
