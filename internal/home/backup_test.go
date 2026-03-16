package home

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestBackupFileName(t *testing.T) {
	name := BackupFileName()
	if name == "" {
		t.Fatal("BackupFileName() returned empty")
	}
	if len(name) < 20 {
		t.Errorf("BackupFileName() = %q, too short", name)
	}
	// Should match ari-backup-YYYYMMDD-HHMMSS.sql.gz
	if name[:11] != "ari-backup-" {
		t.Errorf("BackupFileName() prefix = %q, want ari-backup-", name[:11])
	}
}

func TestBackupRetention_RemovesOldFiles(t *testing.T) {
	dir := t.TempDir()

	// Create old backup files
	oldTime := time.Now().Add(-31 * 24 * time.Hour)
	for _, name := range []string{
		"ari-backup-20251201-120000.sql.gz",
		"ari-backup-20251202-120000.sql.gz",
	} {
		path := filepath.Join(dir, name)
		os.WriteFile(path, []byte("old"), 0644)
		os.Chtimes(path, oldTime, oldTime)
	}

	// Create recent backup
	recentPath := filepath.Join(dir, "ari-backup-20260317-120000.sql.gz")
	os.WriteFile(recentPath, []byte("recent"), 0644)

	removed, err := CleanupOldBackups(dir, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("CleanupOldBackups() error: %v", err)
	}

	if removed != 2 {
		t.Errorf("removed = %d, want 2", removed)
	}

	// Recent should still exist
	if _, err := os.Stat(recentPath); err != nil {
		t.Error("recent backup was deleted")
	}
}

func TestBackupRetention_KeepsRecentFiles(t *testing.T) {
	dir := t.TempDir()

	recentPath := filepath.Join(dir, "ari-backup-20260317-120000.sql.gz")
	os.WriteFile(recentPath, []byte("recent"), 0644)

	removed, err := CleanupOldBackups(dir, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("CleanupOldBackups() error: %v", err)
	}

	if removed != 0 {
		t.Errorf("removed = %d, want 0", removed)
	}
}

func TestBackupRetention_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	removed, err := CleanupOldBackups(dir, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("CleanupOldBackups() error: %v", err)
	}

	if removed != 0 {
		t.Errorf("removed = %d, want 0", removed)
	}
}

func TestBackupService_RunsOnInterval(t *testing.T) {
	var count atomic.Int32
	svc := &BackupService{
		interval:  50 * time.Millisecond,
		retention: 30 * 24 * time.Hour,
		backupDir: t.TempDir(),
		backupFn: func(ctx context.Context) error {
			count.Add(1)
			return nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	svc.Start(ctx)

	if c := count.Load(); c < 2 {
		t.Errorf("backup ran %d times, want at least 2", c)
	}
}

func TestBackupService_GracefulShutdown(t *testing.T) {
	svc := &BackupService{
		interval:  10 * time.Millisecond,
		retention: 30 * 24 * time.Hour,
		backupDir: t.TempDir(),
		backupFn: func(ctx context.Context) error {
			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		svc.Start(ctx)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Good, exited cleanly
	case <-time.After(2 * time.Second):
		t.Fatal("BackupService did not shut down gracefully")
	}
}
