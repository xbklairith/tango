package home

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInitHomeDir_CreatesAllDirectories(t *testing.T) {
	root := t.TempDir()

	if err := InitHomeDir(root); err != nil {
		t.Fatalf("InitHomeDir() error: %v", err)
	}

	dirs := []string{"db", "logs", "secrets", "data", "data/backups", "data/storage", "data/storage/runs", "workspaces"}
	for _, d := range dirs {
		path := filepath.Join(root, d)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("directory %q not created: %v", d, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%q is not a directory", d)
		}
	}

	// Check secrets dir permissions
	info, _ := os.Stat(filepath.Join(root, "secrets"))
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Errorf("secrets dir permissions = %o, want 0700", perm)
	}
}

func TestInitHomeDir_CreatesMasterKey(t *testing.T) {
	root := t.TempDir()

	if err := InitHomeDir(root); err != nil {
		t.Fatalf("InitHomeDir() error: %v", err)
	}

	keyPath := filepath.Join(root, "secrets", "master.key")
	data, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("master.key not created: %v", err)
	}
	if len(data) != 32 {
		t.Errorf("master.key length = %d, want 32", len(data))
	}

	info, _ := os.Stat(keyPath)
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("master.key permissions = %o, want 0600", perm)
	}
}

func TestInitHomeDir_SkipsExistingMasterKey(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "secrets"), 0700)

	existing := []byte("existing-key-do-not-overwrite!!!")
	keyPath := filepath.Join(root, "secrets", "master.key")
	os.WriteFile(keyPath, existing, 0600)

	if err := InitHomeDir(root); err != nil {
		t.Fatalf("InitHomeDir() error: %v", err)
	}

	data, _ := os.ReadFile(keyPath)
	if string(data) != string(existing) {
		t.Errorf("master.key was overwritten; got %q, want %q", data, existing)
	}
}

func TestInitHomeDir_CreatesDefaultConfig(t *testing.T) {
	root := t.TempDir()

	if err := InitHomeDir(root); err != nil {
		t.Fatalf("InitHomeDir() error: %v", err)
	}

	cfgPath := filepath.Join(root, "config.json")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("config.json not created: %v", err)
	}

	var cfg FileConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("config.json is not valid JSON: %v", err)
	}

	if cfg.Server == nil || cfg.Server.Port != 3100 {
		t.Errorf("config.json server port = %v, want 3100", cfg.Server)
	}
}

func TestInitHomeDir_SkipsExistingConfig(t *testing.T) {
	root := t.TempDir()

	customCfg := `{"server": {"port": 9999}}`
	os.WriteFile(filepath.Join(root, "config.json"), []byte(customCfg), 0644)

	if err := InitHomeDir(root); err != nil {
		t.Fatalf("InitHomeDir() error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(root, "config.json"))
	var cfg FileConfig
	json.Unmarshal(data, &cfg)

	if cfg.Server.Port != 9999 {
		t.Errorf("config.json was overwritten; port = %d, want 9999", cfg.Server.Port)
	}
}

func TestInitHomeDir_CreatesEnvFile(t *testing.T) {
	root := t.TempDir()

	if err := InitHomeDir(root); err != nil {
		t.Fatalf("InitHomeDir() error: %v", err)
	}

	envPath := filepath.Join(root, ".env")
	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf(".env not created: %v", err)
	}

	content := string(data)
	if len(content) < 64 {
		t.Errorf(".env content too short: %d chars", len(content))
	}

	info, _ := os.Stat(envPath)
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf(".env permissions = %o, want 0600", perm)
	}
}

func TestInitHomeDir_SkipsExistingEnvFile(t *testing.T) {
	root := t.TempDir()

	existing := "ARI_JWT_SECRET=my-existing-secret\n"
	os.WriteFile(filepath.Join(root, ".env"), []byte(existing), 0600)

	if err := InitHomeDir(root); err != nil {
		t.Fatalf("InitHomeDir() error: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(root, ".env"))
	if string(data) != existing {
		t.Errorf(".env was overwritten")
	}
}

func TestInitHomeDir_Idempotent(t *testing.T) {
	root := t.TempDir()

	if err := InitHomeDir(root); err != nil {
		t.Fatalf("first InitHomeDir() error: %v", err)
	}

	// Read key from first init
	key1, _ := os.ReadFile(filepath.Join(root, "secrets", "master.key"))

	if err := InitHomeDir(root); err != nil {
		t.Fatalf("second InitHomeDir() error: %v", err)
	}

	// Key should not change
	key2, _ := os.ReadFile(filepath.Join(root, "secrets", "master.key"))
	if string(key1) != string(key2) {
		t.Error("master.key changed after second init")
	}
}
