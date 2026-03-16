package claude

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed data/skills/.claude/skills/ari/SKILL.md
var skillContent string

// buildSkillsDir creates a temp directory with the embedded Ari skill content
// for a single run. Returns the directory path and a cleanup function.
func buildSkillsDir() (string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "ari-skills-*")
	if err != nil {
		return "", nil, fmt.Errorf("creating skills temp dir: %w", err)
	}

	skillDir := filepath.Join(tmpDir, ".claude", "skills", "ari")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		os.RemoveAll(tmpDir)
		return "", nil, fmt.Errorf("creating skill dir: %w", err)
	}

	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		os.RemoveAll(tmpDir)
		return "", nil, fmt.Errorf("writing SKILL.md: %w", err)
	}

	return tmpDir, func() { os.RemoveAll(tmpDir) }, nil
}
