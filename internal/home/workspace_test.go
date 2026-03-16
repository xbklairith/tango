package home

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureAgentWorkspace_CreatesDir(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "workspaces"), 0700)

	dir, err := EnsureAgentWorkspace(filepath.Join(root, "workspaces"), "agent-001")
	if err != nil {
		t.Fatalf("EnsureAgentWorkspace() error: %v", err)
	}

	want := filepath.Join(root, "workspaces", "agent-001")
	if dir != want {
		t.Errorf("dir = %q, want %q", dir, want)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("workspace not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("workspace is not a directory")
	}
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Errorf("workspace permissions = %o, want 0700", perm)
	}
}

func TestEnsureAgentWorkspace_Idempotent(t *testing.T) {
	root := t.TempDir()
	wsDir := filepath.Join(root, "workspaces")
	os.MkdirAll(wsDir, 0700)

	_, err := EnsureAgentWorkspace(wsDir, "agent-001")
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}

	_, err = EnsureAgentWorkspace(wsDir, "agent-001")
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}
}

func TestEnsureAgentWorkspace_InvalidAgentID(t *testing.T) {
	root := t.TempDir()
	wsDir := filepath.Join(root, "workspaces")
	os.MkdirAll(wsDir, 0700)

	invalid := []string{"../escape", "", "foo/bar", "..", "a b"}
	for _, id := range invalid {
		_, err := EnsureAgentWorkspace(wsDir, id)
		if err == nil {
			t.Errorf("EnsureAgentWorkspace(%q) should return error", id)
		}
	}
}

func TestCleanupAgentWorkspace_RemovesDir(t *testing.T) {
	root := t.TempDir()
	wsDir := filepath.Join(root, "workspaces")
	os.MkdirAll(wsDir, 0700)

	dir, _ := EnsureAgentWorkspace(wsDir, "agent-001")

	// Put a file in the workspace
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0644)

	if err := CleanupAgentWorkspace(wsDir, "agent-001"); err != nil {
		t.Fatalf("CleanupAgentWorkspace() error: %v", err)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("workspace should be removed")
	}
}

func TestCleanupAgentWorkspace_NonExistent(t *testing.T) {
	root := t.TempDir()
	wsDir := filepath.Join(root, "workspaces")
	os.MkdirAll(wsDir, 0700)

	err := CleanupAgentWorkspace(wsDir, "nonexistent")
	if err != nil {
		t.Fatalf("CleanupAgentWorkspace() should not error for nonexistent: %v", err)
	}
}
