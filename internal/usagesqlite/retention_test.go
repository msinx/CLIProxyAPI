package usagesqlite

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDeleteBeforeRemovesOnlyOldEvents(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTestStore(t, ctx)
	oldTime := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	for _, event := range []Event{
		{EventKey: "old", Timestamp: oldTime, CreatedAt: oldTime},
		{EventKey: "new", Timestamp: newTime, CreatedAt: newTime},
	} {
		if err := store.InsertEvent(ctx, event); err != nil {
			t.Fatalf("InsertEvent(%s) error = %v", event.EventKey, err)
		}
	}

	deleted, err := store.DeleteBefore(ctx, time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("DeleteBefore() error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	page, err := store.ListEvents(ctx, QueryFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if page.TotalCount != 1 || page.Events[0].Timestamp != newTime {
		t.Fatalf("remaining events = %+v, total=%d", page.Events, page.TotalCount)
	}
}

func TestRunMaintenanceDeletesOldEventsAndCreatesBackup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTestStore(t, ctx)
	oldTime := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	if err := store.InsertEvents(ctx, []Event{
		{EventKey: "old", Timestamp: oldTime, CreatedAt: oldTime},
		{EventKey: "new", Timestamp: newTime, CreatedAt: newTime},
	}); err != nil {
		t.Fatalf("InsertEvents() error = %v", err)
	}

	backupDir := filepath.Join(t.TempDir(), "backups")
	result, err := RunMaintenance(ctx, store, MaintenanceOptions{
		Now:                 time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC),
		RetentionDays:       30,
		BackupEnabled:       true,
		BackupDir:           backupDir,
		BackupRetentionDays: 7,
	})
	if err != nil {
		t.Fatalf("RunMaintenance() error = %v", err)
	}
	if result.DeletedEvents != 1 {
		t.Fatalf("DeletedEvents = %d, want 1", result.DeletedEvents)
	}
	if result.BackupPath == "" {
		t.Fatalf("BackupPath is empty")
	}
	if !strings.HasPrefix(result.BackupPath, backupDir) {
		t.Fatalf("BackupPath = %q, want under %q", result.BackupPath, backupDir)
	}
	if _, errStat := os.Stat(result.BackupPath); errStat != nil {
		t.Fatalf("backup stat error = %v", errStat)
	}

	backupDB, err := OpenSQLite(ctx, result.BackupPath)
	if err != nil {
		t.Fatalf("OpenSQLite(backup) error = %v", err)
	}
	t.Cleanup(func() {
		if errClose := backupDB.Close(); errClose != nil {
			t.Fatalf("backup Close() error = %v", errClose)
		}
	})
	backupStore := NewStore(backupDB)
	page, err := backupStore.ListEvents(ctx, QueryFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListEvents(backup) error = %v", err)
	}
	if page.TotalCount != 1 || page.Events[0].RequestID != "" || !page.Events[0].Timestamp.Equal(newTime) {
		t.Fatalf("backup events = %+v, total=%d", page.Events, page.TotalCount)
	}
}

func TestRunMaintenancePrunesExpiredBackups(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTestStore(t, ctx)
	backupDir := t.TempDir()
	oldBackup := filepath.Join(backupDir, "usage-backup-20260401000000.db")
	if err := os.WriteFile(oldBackup, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile(old backup) error = %v", err)
	}
	oldModTime := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(oldBackup, oldModTime, oldModTime); err != nil {
		t.Fatalf("Chtimes(old backup) error = %v", err)
	}

	_, err := RunMaintenance(ctx, store, MaintenanceOptions{
		Now:                 time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC),
		BackupEnabled:       false,
		BackupDir:           backupDir,
		BackupRetentionDays: 7,
	})
	if err != nil {
		t.Fatalf("RunMaintenance() error = %v", err)
	}
	if _, errStat := os.Stat(oldBackup); !os.IsNotExist(errStat) {
		t.Fatalf("old backup still exists or stat failed unexpectedly: %v", errStat)
	}
}
