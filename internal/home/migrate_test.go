package home

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectLegacyDir_Exists(t *testing.T) {
	dir := t.TempDir()
	legacyDir := filepath.Join(dir, "data")
	os.MkdirAll(filepath.Join(legacyDir, "postgres"), 0755)

	if !DetectLegacyDir(legacyDir) {
		t.Error("DetectLegacyDir should return true for dir with postgres/")
	}
}

func TestDetectLegacyDir_NotExists(t *testing.T) {
	dir := t.TempDir()

	if DetectLegacyDir(filepath.Join(dir, "data")) {
		t.Error("DetectLegacyDir should return false for nonexistent dir")
	}
}

func TestDetectLegacyDir_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "data"), 0755)

	if DetectLegacyDir(filepath.Join(dir, "data")) {
		t.Error("DetectLegacyDir should return false for empty data dir")
	}
}

func TestPlanMigration_AllItems(t *testing.T) {
	src := t.TempDir()
	// Create legacy structure
	os.MkdirAll(filepath.Join(src, "postgres"), 0755)
	os.WriteFile(filepath.Join(src, "postgres", "PG_VERSION"), []byte("16"), 0644)
	os.MkdirAll(filepath.Join(src, "pg-runtime"), 0755)
	os.WriteFile(filepath.Join(src, "master.key"), []byte("keydata"), 0600)
	os.MkdirAll(filepath.Join(src, "secrets"), 0700)
	os.WriteFile(filepath.Join(src, "secrets", "jwt.key"), []byte("jwtkey"), 0600)
	os.MkdirAll(filepath.Join(src, "runs"), 0755)
	os.WriteFile(filepath.Join(src, "runs", "run-1.jsonl"), []byte("{}"), 0644)

	target := t.TempDir()
	plan, err := PlanMigration(src, target)
	if err != nil {
		t.Fatalf("PlanMigration() error: %v", err)
	}

	if len(plan.Items) == 0 {
		t.Fatal("plan has no items")
	}

	// Should include: postgres, pg-runtime, master.key, secrets/jwt.key, runs
	types := map[string]bool{}
	for _, item := range plan.Items {
		types[item.Description] = true
	}

	for _, want := range []string{"database", "database runtime", "master key", "jwt key", "run logs"} {
		if !types[want] {
			t.Errorf("plan missing item: %q", want)
		}
	}
}

func TestPlanMigration_PartialItems(t *testing.T) {
	src := t.TempDir()
	// Only create postgres dir
	os.MkdirAll(filepath.Join(src, "postgres"), 0755)

	target := t.TempDir()
	plan, err := PlanMigration(src, target)
	if err != nil {
		t.Fatalf("PlanMigration() error: %v", err)
	}

	if len(plan.Items) != 1 {
		t.Errorf("plan has %d items, want 1", len(plan.Items))
	}
}

func TestExecuteMigration_MovesDB(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "postgres"), 0755)
	os.WriteFile(filepath.Join(src, "postgres", "PG_VERSION"), []byte("16"), 0644)

	target := t.TempDir()
	plan, _ := PlanMigration(src, target)

	result, err := ExecuteMigration(plan, false)
	if err != nil {
		t.Fatalf("ExecuteMigration() error: %v", err)
	}

	if result.FilesMoved != 1 {
		t.Errorf("FilesMoved = %d, want 1", result.FilesMoved)
	}

	// Check destination
	if _, err := os.Stat(filepath.Join(target, "db", "postgres", "PG_VERSION")); err != nil {
		t.Error("PG_VERSION not at target db/postgres/")
	}
}

func TestExecuteMigration_MovesMasterKey(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "master.key"), []byte("keydata"), 0600)

	target := t.TempDir()
	plan, _ := PlanMigration(src, target)

	_, err := ExecuteMigration(plan, false)
	if err != nil {
		t.Fatalf("ExecuteMigration() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(target, "secrets", "master.key"))
	if err != nil {
		t.Fatal("master.key not at target secrets/")
	}
	if string(data) != "keydata" {
		t.Errorf("master.key content = %q", data)
	}
}

func TestExecuteMigration_AlreadyMigrated(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "postgres"), 0755)

	target := t.TempDir()
	// Pre-populate target with data
	os.MkdirAll(filepath.Join(target, "db", "postgres"), 0755)
	os.WriteFile(filepath.Join(target, "db", "postgres", "PG_VERSION"), []byte("existing"), 0644)

	plan, _ := PlanMigration(src, target)

	_, err := ExecuteMigration(plan, false)
	if err == nil {
		t.Fatal("ExecuteMigration() should error when target already has data")
	}
}

func TestExecuteMigration_DryRun(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "postgres"), 0755)
	os.WriteFile(filepath.Join(src, "postgres", "PG_VERSION"), []byte("16"), 0644)

	target := t.TempDir()
	plan, _ := PlanMigration(src, target)

	result, err := ExecuteMigration(plan, true)
	if err != nil {
		t.Fatalf("ExecuteMigration(dryRun) error: %v", err)
	}

	if !result.DryRun {
		t.Error("expected DryRun = true")
	}

	// Source should still exist
	if _, err := os.Stat(filepath.Join(src, "postgres", "PG_VERSION")); err != nil {
		t.Error("source was modified during dry run")
	}

	// Target should NOT have data
	if _, err := os.Stat(filepath.Join(target, "db", "postgres")); !os.IsNotExist(err) {
		t.Error("target should not have data after dry run")
	}
}
