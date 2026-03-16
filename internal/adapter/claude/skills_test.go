package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildSkillsDir(t *testing.T) {
	dir, cleanup, err := buildSkillsDir()
	if err != nil {
		t.Fatalf("buildSkillsDir() error: %v", err)
	}
	defer cleanup()

	if dir == "" {
		t.Fatal("dir is empty")
	}

	// Verify SKILL.md exists
	skillPath := filepath.Join(dir, ".claude", "skills", "ari", "SKILL.md")
	data, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("SKILL.md not found: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "Ari Agent Skill") {
		t.Error("SKILL.md missing expected content")
	}
	if !strings.Contains(content, "ARI_API_KEY") {
		t.Error("SKILL.md missing ARI_API_KEY reference")
	}
	if !strings.Contains(content, "heartbeat") {
		t.Error("SKILL.md missing heartbeat procedure")
	}
}

func TestBuildSkillsDir_Cleanup(t *testing.T) {
	dir, cleanup, err := buildSkillsDir()
	if err != nil {
		t.Fatal(err)
	}

	// Verify dir exists
	if _, err := os.Stat(dir); err != nil {
		t.Fatal("dir should exist before cleanup")
	}

	cleanup()

	// Verify dir removed
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("dir should be removed after cleanup")
	}
}

func TestSkillContent_Embedded(t *testing.T) {
	if skillContent == "" {
		t.Fatal("skillContent is empty — embed failed")
	}
	if !strings.Contains(skillContent, "# Ari Agent Skill") {
		t.Error("skillContent missing header")
	}
}
