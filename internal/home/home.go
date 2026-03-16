// Package home resolves all filesystem paths for an Ari realm.
// It supports multi-realm layouts under ~/.ari/realms/{id}/ and
// allows overrides via ARI_HOME and ARI_REALM_ID environment variables.
package home

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	DefaultRealmID = "default"
	ConfigFileName = "config.json"
)

// realmIDRe allows alphanumeric, underscore, hyphen; must start with letter or digit.
var realmIDRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

// pathSegmentRe validates a safe filesystem path segment (for agent IDs etc.).
var pathSegmentRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// Paths holds all resolved filesystem paths for an Ari realm.
type Paths struct {
	HomeDir       string // ~/.ari
	RealmID       string // "default"
	RealmRoot     string // ~/.ari/realms/default
	ConfigPath    string // ~/.ari/realms/default/config.json
	DBDir         string // ~/.ari/realms/default/db
	LogsDir       string // ~/.ari/realms/default/logs
	SecretsDir    string // ~/.ari/realms/default/secrets
	BackupDir     string // ~/.ari/realms/default/data/backups
	StorageDir    string // ~/.ari/realms/default/data/storage
	WorkspacesDir string // ~/.ari/realms/default/workspaces
}

// Resolve computes all paths from environment variables and defaults.
func Resolve() (*Paths, error) {
	homeDir := resolveHomeDir()
	realmID := resolveRealmID()

	if err := ValidateRealmID(realmID); err != nil {
		return nil, fmt.Errorf("invalid ARI_REALM_ID: %w", err)
	}

	root := filepath.Join(homeDir, "realms", realmID)

	return &Paths{
		HomeDir:       homeDir,
		RealmID:       realmID,
		RealmRoot:     root,
		ConfigPath:    filepath.Join(root, ConfigFileName),
		DBDir:         filepath.Join(root, "db"),
		LogsDir:       filepath.Join(root, "logs"),
		SecretsDir:    filepath.Join(root, "secrets"),
		BackupDir:     filepath.Join(root, "data", "backups"),
		StorageDir:    filepath.Join(root, "data", "storage"),
		WorkspacesDir: filepath.Join(root, "workspaces"),
	}, nil
}

// MasterKeyPath returns the path to the master encryption key.
func (p *Paths) MasterKeyPath() string {
	return filepath.Join(p.SecretsDir, "master.key")
}

// JWTKeyPath returns the path to the JWT signing key.
func (p *Paths) JWTKeyPath() string {
	return filepath.Join(p.SecretsDir, "jwt.key")
}

// RunLogPath returns the path for a specific run's JSONL log file.
// Validates runID to prevent path traversal.
func (p *Paths) RunLogPath(runID string) (string, error) {
	id := strings.TrimSpace(runID)
	if !pathSegmentRe.MatchString(id) {
		return "", fmt.Errorf("invalid run ID for log path: %q", runID)
	}
	return filepath.Join(p.StorageDir, "runs", id+".jsonl"), nil
}

// AgentWorkspaceDir returns the workspace directory for a specific agent.
// Validates agentID to prevent path traversal.
func (p *Paths) AgentWorkspaceDir(agentID string) (string, error) {
	id := strings.TrimSpace(agentID)
	if !pathSegmentRe.MatchString(id) {
		return "", fmt.Errorf("invalid agent ID for workspace path: %q", agentID)
	}
	if strings.Contains(id, "..") {
		return "", fmt.Errorf("invalid agent ID for workspace path: %q", agentID)
	}
	return filepath.Join(p.WorkspacesDir, id), nil
}

// ValidateRealmID checks that a realm ID is safe for use as a directory name.
func ValidateRealmID(id string) error {
	if id == "" {
		return fmt.Errorf("realm ID must not be empty")
	}
	if strings.Contains(id, "..") {
		return fmt.Errorf("realm ID must not contain '..'")
	}
	if !realmIDRe.MatchString(id) {
		return fmt.Errorf("realm ID %q must match %s", id, realmIDRe.String())
	}
	return nil
}

func resolveHomeDir() string {
	if v := os.Getenv("ARI_HOME"); v != "" {
		return expandHomePrefix(v)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ari")
}

func resolveRealmID() string {
	if v := os.Getenv("ARI_REALM_ID"); v != "" {
		return v
	}
	return DefaultRealmID
}

func expandHomePrefix(value string) string {
	if value == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if strings.HasPrefix(value, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, value[2:])
	}
	abs, _ := filepath.Abs(value)
	return abs
}
