package home

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BackupFileName generates a timestamped backup filename.
func BackupFileName() string {
	return fmt.Sprintf("ari-backup-%s.sql.gz", time.Now().Format("20060102-150405"))
}

// CleanupOldBackups removes backup files older than the retention duration.
// Returns the number of files removed.
func CleanupOldBackups(dir string, retention time.Duration) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("reading backup dir: %w", err)
	}

	cutoff := time.Now().Add(-retention)
	removed := 0

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "ari-backup-") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
				slog.Warn("failed to remove old backup", "file", entry.Name(), "error", err)
				continue
			}
			removed++
		}
	}

	return removed, nil
}

// BackupService runs periodic database backups.
type BackupService struct {
	interval  time.Duration
	retention time.Duration
	backupDir string
	backupFn  func(ctx context.Context) error // injectable for testing
}

// NewBackupService creates a backup service with the given config.
func NewBackupService(interval, retention time.Duration, backupDir string, backupFn func(ctx context.Context) error) *BackupService {
	return &BackupService{
		interval:  interval,
		retention: retention,
		backupDir: backupDir,
		backupFn:  backupFn,
	}
}

// Start runs the backup loop. Blocks until ctx is cancelled.
func (s *BackupService) Start(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.backupFn(ctx); err != nil {
				slog.Error("backup failed", "error", err)
			} else {
				slog.Info("backup completed")
			}
			if _, err := CleanupOldBackups(s.backupDir, s.retention); err != nil {
				slog.Error("backup cleanup failed", "error", err)
			}
		}
	}
}
