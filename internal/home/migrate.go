package home

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// MigrationItem represents a single item to migrate.
type MigrationItem struct {
	Source      string
	Dest        string
	IsDir       bool
	Description string
}

// MigrationPlan holds the list of items to migrate.
type MigrationPlan struct {
	SourceDir string
	TargetDir string
	Items     []MigrationItem
}

// MigrationResult holds the outcome of a migration.
type MigrationResult struct {
	FilesMoved int
	DryRun     bool
}

// DetectLegacyDir checks if a directory looks like a legacy Ari data dir.
func DetectLegacyDir(dir string) bool {
	return isLegacyDataDir(dir)
}

// PlanMigration builds a migration plan from a legacy data dir to a new realm root.
func PlanMigration(sourceDir, targetDir string) (*MigrationPlan, error) {
	plan := &MigrationPlan{
		SourceDir: sourceDir,
		TargetDir: targetDir,
	}

	// Map legacy paths to new structure
	mappings := []struct {
		src  string
		dst  string
		dir  bool
		desc string
	}{
		{"postgres", filepath.Join("db", "postgres"), true, "database"},
		{"pg-runtime", filepath.Join("db", "pg-runtime"), true, "database runtime"},
		{"master.key", filepath.Join("secrets", "master.key"), false, "master key"},
		{filepath.Join("secrets", "jwt.key"), filepath.Join("secrets", "jwt.key"), false, "jwt key"},
		{"runs", filepath.Join("data", "storage", "runs"), true, "run logs"},
	}

	for _, m := range mappings {
		srcPath := filepath.Join(sourceDir, m.src)
		if _, err := os.Stat(srcPath); err != nil {
			continue // item doesn't exist in source
		}
		plan.Items = append(plan.Items, MigrationItem{
			Source:      srcPath,
			Dest:        filepath.Join(targetDir, m.dst),
			IsDir:       m.dir,
			Description: m.desc,
		})
	}

	return plan, nil
}

// ExecuteMigration executes or dry-runs a migration plan.
func ExecuteMigration(plan *MigrationPlan, dryRun bool) (*MigrationResult, error) {
	result := &MigrationResult{DryRun: dryRun}

	if dryRun {
		result.FilesMoved = len(plan.Items)
		return result, nil
	}

	// Check if target already has data
	for _, item := range plan.Items {
		if _, err := os.Stat(item.Dest); err == nil {
			return nil, fmt.Errorf("target already exists: %s — aborting to prevent data loss", item.Dest)
		}
	}

	for _, item := range plan.Items {
		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(item.Dest), 0700); err != nil {
			return nil, fmt.Errorf("creating target dir for %s: %w", item.Description, err)
		}

		if item.IsDir {
			if err := copyDir(item.Source, item.Dest); err != nil {
				return nil, fmt.Errorf("copying %s: %w", item.Description, err)
			}
		} else {
			if err := copyFile(item.Source, item.Dest); err != nil {
				return nil, fmt.Errorf("copying %s: %w", item.Description, err)
			}
		}

		result.FilesMoved++
	}

	return result, nil
}

// sanitizePerm ensures sensitive files get restricted permissions.
func sanitizePerm(path string, mode os.FileMode) os.FileMode {
	base := filepath.Base(path)
	if base == "master.key" || base == "jwt.key" || base == ".env" {
		return 0600
	}
	return mode
}

func copyFile(src, dst string) error {
	// Use Lstat to detect symlinks — do not follow them
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to copy symlink: %s", src)
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	perm := sanitizePerm(dst, info.Mode().Perm())
	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip symlinks to prevent symlink attacks
		if d.Type()&os.ModeSymlink != 0 {
			slog.Warn("skipping symlink during migration", "path", path)
			return nil
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0700)
		}

		return copyFile(path, target)
	})
}
