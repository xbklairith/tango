package home

import (
	"log/slog"
	"os"
	"path/filepath"
)

// ResolveDataDir determines the data directory to use based on priority:
// 1. ARI_DATA_DIR env var (explicit override, highest priority)
// 2. Home directory realm root (if it exists)
// 3. Legacy ./data/ directory (if it exists and home does not)
// 4. Home directory realm root (default — triggers auto-init)
//
// Returns the resolved directory and whether legacy mode is active.
func ResolveDataDir(paths *Paths, cwd string) (string, bool) {
	// Explicit override takes highest priority
	if v := os.Getenv("ARI_DATA_DIR"); v != "" {
		return v, false
	}

	// Check if home realm root exists
	homeExists := dirExists(paths.RealmRoot)

	// Check if legacy ./data/ exists with DB artifacts
	legacyDir := filepath.Join(cwd, "data")
	legacyExists := isLegacyDataDir(legacyDir)

	if homeExists {
		if legacyExists {
			slog.Info("both home and legacy data directories exist; using home",
				"home", paths.RealmRoot, "legacy", legacyDir)
		}
		return paths.RealmRoot, false
	}

	if legacyExists {
		slog.Warn("using legacy ./data/ directory — run 'ari migrate-home' to upgrade",
			"legacy", legacyDir)
		return legacyDir, true
	}

	// Default: use home (will be auto-initialized)
	return paths.RealmRoot, false
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isLegacyDataDir(dir string) bool {
	// Check for postgres subdirectory as the marker for a real legacy data dir
	if dirExists(filepath.Join(dir, "postgres")) {
		return true
	}
	// Also check for pg-runtime as alternate marker
	if dirExists(filepath.Join(dir, "pg-runtime")) {
		return true
	}
	return false
}
