package home

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHomeDir_Default(t *testing.T) {
	t.Setenv("ARI_HOME", "")
	os.Unsetenv("ARI_HOME")

	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	paths, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	want := filepath.Join(userHome, ".ari")
	if paths.HomeDir != want {
		t.Errorf("HomeDir = %q, want %q", paths.HomeDir, want)
	}
}

func TestHomeDir_ARI_HOME_Override(t *testing.T) {
	t.Setenv("ARI_HOME", "/tmp/custom-ari")
	t.Setenv("ARI_INSTANCE_ID", "")
	os.Unsetenv("ARI_INSTANCE_ID")

	paths, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	if paths.HomeDir != "/tmp/custom-ari" {
		t.Errorf("HomeDir = %q, want %q", paths.HomeDir, "/tmp/custom-ari")
	}
}

func TestHomeDir_TildeExpansion(t *testing.T) {
	userHome, _ := os.UserHomeDir()
	t.Setenv("ARI_HOME", "~/my-ari")
	t.Setenv("ARI_INSTANCE_ID", "")
	os.Unsetenv("ARI_INSTANCE_ID")

	paths, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	want := filepath.Join(userHome, "my-ari")
	if paths.HomeDir != want {
		t.Errorf("HomeDir = %q, want %q", paths.HomeDir, want)
	}
}

func TestInstanceRoot_Default(t *testing.T) {
	t.Setenv("ARI_HOME", "/tmp/test-ari")
	t.Setenv("ARI_INSTANCE_ID", "")
	os.Unsetenv("ARI_INSTANCE_ID")

	paths, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	want := "/tmp/test-ari/instances/default"
	if paths.InstanceRoot != want {
		t.Errorf("InstanceRoot = %q, want %q", paths.InstanceRoot, want)
	}
	if paths.InstanceID != "default" {
		t.Errorf("InstanceID = %q, want %q", paths.InstanceID, "default")
	}
}

func TestInstanceRoot_CustomID(t *testing.T) {
	t.Setenv("ARI_HOME", "/tmp/test-ari")
	t.Setenv("ARI_INSTANCE_ID", "staging")

	paths, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	want := "/tmp/test-ari/instances/staging"
	if paths.InstanceRoot != want {
		t.Errorf("InstanceRoot = %q, want %q", paths.InstanceRoot, want)
	}
	if paths.InstanceID != "staging" {
		t.Errorf("InstanceID = %q, want %q", paths.InstanceID, "staging")
	}
}

func TestInstanceRoot_ARI_HOME_And_InstanceID(t *testing.T) {
	t.Setenv("ARI_HOME", "/opt/ari")
	t.Setenv("ARI_INSTANCE_ID", "prod")

	paths, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	want := "/opt/ari/instances/prod"
	if paths.InstanceRoot != want {
		t.Errorf("InstanceRoot = %q, want %q", paths.InstanceRoot, want)
	}
}

func TestSubdirectories(t *testing.T) {
	t.Setenv("ARI_HOME", "/tmp/test-ari")
	t.Setenv("ARI_INSTANCE_ID", "")
	os.Unsetenv("ARI_INSTANCE_ID")

	paths, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	root := "/tmp/test-ari/instances/default"

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"ConfigPath", paths.ConfigPath, filepath.Join(root, "config.json")},
		{"DBDir", paths.DBDir, filepath.Join(root, "db")},
		{"LogsDir", paths.LogsDir, filepath.Join(root, "logs")},
		{"SecretsDir", paths.SecretsDir, filepath.Join(root, "secrets")},
		{"BackupDir", paths.BackupDir, filepath.Join(root, "data", "backups")},
		{"StorageDir", paths.StorageDir, filepath.Join(root, "data", "storage")},
		{"WorkspacesDir", paths.WorkspacesDir, filepath.Join(root, "workspaces")},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestMasterKeyPath(t *testing.T) {
	t.Setenv("ARI_HOME", "/tmp/test-ari")
	t.Setenv("ARI_INSTANCE_ID", "")
	os.Unsetenv("ARI_INSTANCE_ID")

	paths, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	want := "/tmp/test-ari/instances/default/secrets/master.key"
	if paths.MasterKeyPath() != want {
		t.Errorf("MasterKeyPath() = %q, want %q", paths.MasterKeyPath(), want)
	}
}

func TestJWTKeyPath(t *testing.T) {
	t.Setenv("ARI_HOME", "/tmp/test-ari")
	t.Setenv("ARI_INSTANCE_ID", "")
	os.Unsetenv("ARI_INSTANCE_ID")

	paths, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	want := "/tmp/test-ari/instances/default/secrets/jwt.key"
	if paths.JWTKeyPath() != want {
		t.Errorf("JWTKeyPath() = %q, want %q", paths.JWTKeyPath(), want)
	}
}

func TestAgentWorkspaceDir(t *testing.T) {
	t.Setenv("ARI_HOME", "/tmp/test-ari")
	t.Setenv("ARI_INSTANCE_ID", "")
	os.Unsetenv("ARI_INSTANCE_ID")

	paths, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	dir, err := paths.AgentWorkspaceDir("abc-123")
	if err != nil {
		t.Fatalf("AgentWorkspaceDir() error: %v", err)
	}

	want := "/tmp/test-ari/instances/default/workspaces/abc-123"
	if dir != want {
		t.Errorf("AgentWorkspaceDir() = %q, want %q", dir, want)
	}
}

func TestAgentWorkspaceDir_PathTraversal(t *testing.T) {
	t.Setenv("ARI_HOME", "/tmp/test-ari")
	t.Setenv("ARI_INSTANCE_ID", "")
	os.Unsetenv("ARI_INSTANCE_ID")

	paths, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	invalid := []string{"../escape", "../../etc/passwd", "foo/../bar", "", "foo/bar", " ", "a b"}
	for _, id := range invalid {
		_, err := paths.AgentWorkspaceDir(id)
		if err == nil {
			t.Errorf("AgentWorkspaceDir(%q) should return error for path traversal/invalid ID", id)
		}
	}
}

func TestValidateInstanceID_Valid(t *testing.T) {
	valid := []string{"default", "staging", "my-project-1", "prod_v2", "A", "a1"}
	for _, id := range valid {
		if err := ValidateInstanceID(id); err != nil {
			t.Errorf("ValidateInstanceID(%q) returned error: %v", id, err)
		}
	}
}

func TestValidateInstanceID_Invalid(t *testing.T) {
	invalid := []string{
		"",             // empty
		"a/b",          // contains /
		"a..b",         // contains ..
		"-start",       // starts with -
		strings.Repeat("a", 65), // too long (>64)
		"has space",    // contains space
		"special!@#",   // special characters
		"../escape",    // path traversal
		".hidden",      // starts with .
	}
	for _, id := range invalid {
		if err := ValidateInstanceID(id); err == nil {
			t.Errorf("ValidateInstanceID(%q) should return error", id)
		}
	}
}

func TestResolve_InvalidInstanceID(t *testing.T) {
	t.Setenv("ARI_HOME", "/tmp/test-ari")
	t.Setenv("ARI_INSTANCE_ID", "../escape")

	_, err := Resolve()
	if err == nil {
		t.Fatal("Resolve() should return error for invalid instance ID")
	}
}

func TestRunLogPath(t *testing.T) {
	t.Setenv("ARI_HOME", "/tmp/test-ari")
	t.Setenv("ARI_INSTANCE_ID", "")
	os.Unsetenv("ARI_INSTANCE_ID")

	paths, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	want := "/tmp/test-ari/instances/default/data/storage/runs/run-abc.jsonl"
	if paths.RunLogPath("run-abc") != want {
		t.Errorf("RunLogPath() = %q, want %q", paths.RunLogPath("run-abc"), want)
	}
}
