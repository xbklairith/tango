// Package home resolves all filesystem paths for an Ari instance.
// It supports multi-instance layouts under ~/.ari/instances/{id}/ and
// allows overrides via ARI_HOME and ARI_INSTANCE_ID environment variables.
package home

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	DefaultInstanceID = "default"
	ConfigFileName    = "config.json"
)

// instanceIDRe allows alphanumeric, underscore, hyphen; must start with letter or digit.
var instanceIDRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

// pathSegmentRe validates a safe filesystem path segment (for agent IDs etc.).
var pathSegmentRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// Paths holds all resolved filesystem paths for an Ari instance.
type Paths struct {
	HomeDir       string // ~/.ari
	InstanceID    string // "default"
	InstanceRoot  string // ~/.ari/instances/default
	ConfigPath    string // ~/.ari/instances/default/config.json
	DBDir         string // ~/.ari/instances/default/db
	LogsDir       string // ~/.ari/instances/default/logs
	SecretsDir    string // ~/.ari/instances/default/secrets
	BackupDir     string // ~/.ari/instances/default/data/backups
	StorageDir    string // ~/.ari/instances/default/data/storage
	WorkspacesDir string // ~/.ari/instances/default/workspaces
}

// Resolve computes all paths from environment variables and defaults.
func Resolve() (*Paths, error) {
	homeDir := resolveHomeDir()
	instanceID := resolveInstanceID()

	if err := ValidateInstanceID(instanceID); err != nil {
		return nil, fmt.Errorf("invalid ARI_INSTANCE_ID: %w", err)
	}

	root := filepath.Join(homeDir, "instances", instanceID)

	return &Paths{
		HomeDir:       homeDir,
		InstanceID:    instanceID,
		InstanceRoot:  root,
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
func (p *Paths) RunLogPath(runID string) string {
	return filepath.Join(p.StorageDir, "runs", runID+".jsonl")
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

// ValidateInstanceID checks that an instance ID is safe for use as a directory name.
func ValidateInstanceID(id string) error {
	if id == "" {
		return fmt.Errorf("instance ID must not be empty")
	}
	if strings.Contains(id, "..") {
		return fmt.Errorf("instance ID must not contain '..'")
	}
	if !instanceIDRe.MatchString(id) {
		return fmt.Errorf("instance ID %q must match %s", id, instanceIDRe.String())
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

func resolveInstanceID() string {
	if v := os.Getenv("ARI_INSTANCE_ID"); v != "" {
		return v
	}
	return DefaultInstanceID
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
	return value
}
