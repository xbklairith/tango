package home

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDataDir_NoLegacy_NoHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ARI_HOME", filepath.Join(tmp, "ari-home"))
	t.Setenv("ARI_INSTANCE_ID", "")
	os.Unsetenv("ARI_INSTANCE_ID")
	t.Setenv("ARI_DATA_DIR", "")
	os.Unsetenv("ARI_DATA_DIR")

	cwd := filepath.Join(tmp, "project")
	os.MkdirAll(cwd, 0755)

	paths, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}

	dir, legacy := ResolveDataDir(paths, cwd)
	if legacy {
		t.Error("expected non-legacy mode")
	}
	if dir != paths.InstanceRoot {
		t.Errorf("dir = %q, want %q", dir, paths.InstanceRoot)
	}
}

func TestResolveDataDir_LegacyExists_NoHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ARI_HOME", filepath.Join(tmp, "ari-home"))
	t.Setenv("ARI_INSTANCE_ID", "")
	os.Unsetenv("ARI_INSTANCE_ID")
	t.Setenv("ARI_DATA_DIR", "")
	os.Unsetenv("ARI_DATA_DIR")

	cwd := filepath.Join(tmp, "project")
	os.MkdirAll(cwd, 0755)

	// Create legacy ./data/ dir
	legacyDir := filepath.Join(cwd, "data")
	os.MkdirAll(filepath.Join(legacyDir, "postgres"), 0755)

	paths, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}

	dir, legacy := ResolveDataDir(paths, cwd)
	if !legacy {
		t.Error("expected legacy mode")
	}
	if dir != legacyDir {
		t.Errorf("dir = %q, want %q (legacy)", dir, legacyDir)
	}
}

func TestResolveDataDir_HomeExists(t *testing.T) {
	tmp := t.TempDir()
	homeDir := filepath.Join(tmp, "ari-home")
	t.Setenv("ARI_HOME", homeDir)
	t.Setenv("ARI_INSTANCE_ID", "")
	os.Unsetenv("ARI_INSTANCE_ID")
	t.Setenv("ARI_DATA_DIR", "")
	os.Unsetenv("ARI_DATA_DIR")

	cwd := filepath.Join(tmp, "project")
	os.MkdirAll(cwd, 0755)

	paths, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}

	// Create home instance root
	os.MkdirAll(paths.InstanceRoot, 0700)

	dir, legacy := ResolveDataDir(paths, cwd)
	if legacy {
		t.Error("expected non-legacy mode")
	}
	if dir != paths.InstanceRoot {
		t.Errorf("dir = %q, want %q", dir, paths.InstanceRoot)
	}
}

func TestResolveDataDir_BothExist(t *testing.T) {
	tmp := t.TempDir()
	homeDir := filepath.Join(tmp, "ari-home")
	t.Setenv("ARI_HOME", homeDir)
	t.Setenv("ARI_INSTANCE_ID", "")
	os.Unsetenv("ARI_INSTANCE_ID")
	t.Setenv("ARI_DATA_DIR", "")
	os.Unsetenv("ARI_DATA_DIR")

	cwd := filepath.Join(tmp, "project")
	os.MkdirAll(cwd, 0755)

	paths, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}

	// Create both
	os.MkdirAll(paths.InstanceRoot, 0700)
	os.MkdirAll(filepath.Join(cwd, "data", "postgres"), 0755)

	dir, legacy := ResolveDataDir(paths, cwd)
	if legacy {
		t.Error("home exists, so should not be legacy")
	}
	if dir != paths.InstanceRoot {
		t.Errorf("dir = %q, want %q (home takes priority)", dir, paths.InstanceRoot)
	}
}

func TestResolveDataDir_ARI_DATA_DIR_Override(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ARI_HOME", filepath.Join(tmp, "ari-home"))
	t.Setenv("ARI_INSTANCE_ID", "")
	os.Unsetenv("ARI_INSTANCE_ID")
	t.Setenv("ARI_DATA_DIR", "/custom/override")

	paths, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}

	dir, legacy := ResolveDataDir(paths, tmp)
	if legacy {
		t.Error("explicit ARI_DATA_DIR should not be legacy")
	}
	if dir != "/custom/override" {
		t.Errorf("dir = %q, want /custom/override (explicit override)", dir)
	}
}
