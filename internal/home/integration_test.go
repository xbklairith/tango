package home

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestIntegration_FreshInit_CreatesFullStructure(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ARI_HOME", tmp)
	t.Setenv("ARI_REALM_ID", "")
	os.Unsetenv("ARI_REALM_ID")

	paths, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}

	if err := InitHomeDir(paths.RealmRoot); err != nil {
		t.Fatalf("InitHomeDir() error: %v", err)
	}

	// Verify all directories exist
	dirs := []string{
		filepath.Join(paths.RealmRoot, "db"),
		filepath.Join(paths.RealmRoot, "logs"),
		filepath.Join(paths.RealmRoot, "secrets"),
		filepath.Join(paths.RealmRoot, "data"),
		filepath.Join(paths.RealmRoot, "data", "backups"),
		filepath.Join(paths.RealmRoot, "data", "storage"),
		filepath.Join(paths.RealmRoot, "data", "storage", "runs"),
		filepath.Join(paths.RealmRoot, "workspaces"),
	}
	for _, d := range dirs {
		if _, err := os.Stat(d); err != nil {
			t.Errorf("directory missing: %s", d)
		}
	}

	// Verify config.json
	cfgData, err := os.ReadFile(paths.ConfigPath)
	if err != nil {
		t.Fatalf("config.json missing: %v", err)
	}
	var cfg FileConfig
	if err := json.Unmarshal(cfgData, &cfg); err != nil {
		t.Fatalf("config.json invalid: %v", err)
	}
	if cfg.Server == nil || cfg.Server.Port != 3100 {
		t.Error("config.json missing default server port")
	}

	// Verify master.key
	keyData, err := os.ReadFile(paths.MasterKeyPath())
	if err != nil {
		t.Fatal("master.key missing")
	}
	if len(keyData) != 32 {
		t.Errorf("master.key size = %d, want 32", len(keyData))
	}

	// Verify .env
	envData, err := os.ReadFile(filepath.Join(paths.RealmRoot, ".env"))
	if err != nil {
		t.Fatal(".env missing")
	}
	if len(envData) < 30 {
		t.Error(".env too short")
	}
}

func TestIntegration_LegacyCompat(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ARI_HOME", filepath.Join(tmp, "ari-home"))
	t.Setenv("ARI_REALM_ID", "")
	os.Unsetenv("ARI_REALM_ID")
	t.Setenv("ARI_DATA_DIR", "")
	os.Unsetenv("ARI_DATA_DIR")

	cwd := filepath.Join(tmp, "project")
	os.MkdirAll(cwd, 0755)

	// Create legacy data dir
	legacyDir := filepath.Join(cwd, "data")
	os.MkdirAll(filepath.Join(legacyDir, "postgres"), 0755)
	os.WriteFile(filepath.Join(legacyDir, "postgres", "PG_VERSION"), []byte("16"), 0644)

	paths, _ := Resolve()
	dir, isLegacy := ResolveDataDir(paths, cwd)

	if !isLegacy {
		t.Error("should detect legacy mode")
	}
	if dir != legacyDir {
		t.Errorf("should use legacy dir %q, got %q", legacyDir, dir)
	}
}

func TestIntegration_MigrateHome_FullLifecycle(t *testing.T) {
	tmp := t.TempDir()

	// Set up legacy structure
	legacyDir := filepath.Join(tmp, "legacy-data")
	os.MkdirAll(filepath.Join(legacyDir, "postgres"), 0755)
	os.WriteFile(filepath.Join(legacyDir, "postgres", "PG_VERSION"), []byte("16"), 0644)
	os.WriteFile(filepath.Join(legacyDir, "master.key"), []byte("0123456789abcdef0123456789abcdef"), 0600)
	os.MkdirAll(filepath.Join(legacyDir, "runs"), 0755)
	os.WriteFile(filepath.Join(legacyDir, "runs", "run-1.jsonl"), []byte(`{"msg":"hello"}`), 0644)

	// Target realm root
	targetRoot := filepath.Join(tmp, "ari-home", "realms", "default")

	// Plan
	plan, err := PlanMigration(legacyDir, targetRoot)
	if err != nil {
		t.Fatalf("PlanMigration() error: %v", err)
	}
	if len(plan.Items) != 3 {
		t.Fatalf("plan has %d items, want 3 (postgres, master.key, runs)", len(plan.Items))
	}

	// Execute
	result, err := ExecuteMigration(plan, false)
	if err != nil {
		t.Fatalf("ExecuteMigration() error: %v", err)
	}
	if result.FilesMoved != 3 {
		t.Errorf("FilesMoved = %d, want 3", result.FilesMoved)
	}

	// Verify files at target
	pgVersion, err := os.ReadFile(filepath.Join(targetRoot, "db", "postgres", "PG_VERSION"))
	if err != nil {
		t.Fatal("PG_VERSION not at target")
	}
	if string(pgVersion) != "16" {
		t.Errorf("PG_VERSION = %q", pgVersion)
	}

	masterKey, err := os.ReadFile(filepath.Join(targetRoot, "secrets", "master.key"))
	if err != nil {
		t.Fatal("master.key not at target")
	}
	if string(masterKey) != "0123456789abcdef0123456789abcdef" {
		t.Error("master.key content mismatch")
	}

	runLog, err := os.ReadFile(filepath.Join(targetRoot, "data", "storage", "runs", "run-1.jsonl"))
	if err != nil {
		t.Fatal("run log not at target")
	}
	if string(runLog) != `{"msg":"hello"}` {
		t.Error("run log content mismatch")
	}
}

func TestIntegration_AgentWorkspace_Lifecycle(t *testing.T) {
	tmp := t.TempDir()
	wsDir := filepath.Join(tmp, "workspaces")
	os.MkdirAll(wsDir, 0700)

	// Create workspace
	dir, err := EnsureAgentWorkspace(wsDir, "agent-abc")
	if err != nil {
		t.Fatal(err)
	}

	// Verify
	if _, err := os.Stat(dir); err != nil {
		t.Fatal("workspace not created")
	}

	// Write a file
	os.WriteFile(filepath.Join(dir, "work.txt"), []byte("data"), 0644)

	// Cleanup
	if err := CleanupAgentWorkspace(wsDir, "agent-abc"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("workspace not cleaned up")
	}
}

func TestIntegration_MultiRealm(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ARI_HOME", tmp)

	// Init realm "alpha"
	t.Setenv("ARI_REALM_ID", "alpha")
	pathsAlpha, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if err := InitHomeDir(pathsAlpha.RealmRoot); err != nil {
		t.Fatal(err)
	}

	// Init realm "beta"
	t.Setenv("ARI_REALM_ID", "beta")
	pathsBeta, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if err := InitHomeDir(pathsBeta.RealmRoot); err != nil {
		t.Fatal(err)
	}

	// Verify isolation
	if pathsAlpha.RealmRoot == pathsBeta.RealmRoot {
		t.Error("realms should have different roots")
	}

	// Each should have its own master key
	keyAlpha, _ := os.ReadFile(pathsAlpha.MasterKeyPath())
	keyBeta, _ := os.ReadFile(pathsBeta.MasterKeyPath())
	if string(keyAlpha) == string(keyBeta) {
		t.Error("realms should have different master keys")
	}
}

func TestIntegration_CustomARI_HOME(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "custom-ari")
	t.Setenv("ARI_HOME", custom)
	t.Setenv("ARI_REALM_ID", "")
	os.Unsetenv("ARI_REALM_ID")

	paths, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}

	if paths.HomeDir != custom {
		t.Errorf("HomeDir = %q, want %q", paths.HomeDir, custom)
	}

	if err := InitHomeDir(paths.RealmRoot); err != nil {
		t.Fatal(err)
	}

	// Verify structure under custom path
	if _, err := os.Stat(filepath.Join(paths.RealmRoot, "db")); err != nil {
		t.Error("db dir not created under custom ARI_HOME")
	}
	if _, err := os.Stat(paths.ConfigPath); err != nil {
		t.Error("config.json not created under custom ARI_HOME")
	}
}
