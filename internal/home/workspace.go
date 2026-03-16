package home

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnsureAgentWorkspace creates a workspace directory for the given agent.
// Returns the workspace path. Validates agentID to prevent path traversal.
func EnsureAgentWorkspace(workspacesDir, agentID string) (string, error) {
	id := strings.TrimSpace(agentID)
	if !pathSegmentRe.MatchString(id) {
		return "", fmt.Errorf("invalid agent ID for workspace: %q", agentID)
	}
	if strings.Contains(id, "..") {
		return "", fmt.Errorf("invalid agent ID for workspace: %q", agentID)
	}

	dir := filepath.Join(workspacesDir, id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("creating agent workspace: %w", err)
	}

	return dir, nil
}

// CleanupAgentWorkspace removes the workspace directory for the given agent.
// Returns nil if the directory does not exist.
func CleanupAgentWorkspace(workspacesDir, agentID string) error {
	id := strings.TrimSpace(agentID)
	if !pathSegmentRe.MatchString(id) {
		return fmt.Errorf("invalid agent ID for workspace cleanup: %q", agentID)
	}

	dir := filepath.Join(workspacesDir, id)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}

	return os.RemoveAll(dir)
}
