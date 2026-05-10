package adapter

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/usagekeeper/upstream/backup"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/usagekeeper/upstream/entities"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/usagekeeper/upstream/repository"
	servicedto "github.com/router-for-me/CLIProxyAPI/v7/internal/usagekeeper/upstream/service/dto"
)

func TestRunMaintenanceRemovesOldUsageEventsAndKeepsRecentEvents(t *testing.T) {
	app := newTestAppWithConfig(t, Config{
		Enabled:       true,
		DatabasePath:  filepath.Join(t.TempDir(), "usage.db"),
		BasePath:      "/usage",
		BackupEnabled: false,
		RetentionDays: 7,
	})
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	previousNow := timeNow
	timeNow = func() time.Time { return now }
	t.Cleanup(func() { timeNow = previousNow })

	events := []entities.UsageEvent{
		{EventKey: "old", APIGroupKey: "provider", Model: "old", Timestamp: now.AddDate(0, 0, -8), TotalTokens: 1},
		{EventKey: "recent", APIGroupKey: "provider", Model: "recent", Timestamp: now.AddDate(0, 0, -1), TotalTokens: 1},
	}
	if _, _, err := repository.InsertUsageEvents(app.DB, events); err != nil {
		t.Fatalf("InsertUsageEvents returned error: %v", err)
	}

	if err := app.RunMaintenance(context.Background()); err != nil {
		t.Fatalf("RunMaintenance returned error: %v", err)
	}

	page, err := app.UsageProvider.ListUsageEvents(context.Background(), servicedto.UsageFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListUsageEvents returned error: %v", err)
	}
	if page.TotalCount != 1 || len(page.Events) != 1 || page.Events[0].Model != "recent" {
		t.Fatalf("unexpected events after cleanup: %+v", page)
	}
}

func TestRunMaintenanceCreatesBackupAndRemovesExpiredBackups(t *testing.T) {
	root := t.TempDir()
	app := newTestAppWithConfig(t, Config{
		Enabled:             true,
		DatabasePath:        filepath.Join(root, "usage.db"),
		BasePath:            "/usage",
		BackupEnabled:       true,
		BackupDir:           filepath.Join(root, "backups"),
		BackupRetentionDays: 1,
		RetentionDays:       30,
	})
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	previousNow := timeNow
	timeNow = func() time.Time { return now }
	t.Cleanup(func() { timeNow = previousNow })
	oldDir := filepath.Join(app.Config.BackupDir, "2026-05-01")
	if err := os.MkdirAll(oldDir, 0o700); err != nil {
		t.Fatalf("create old backup dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "database_old.db"), []byte("old"), 0o600); err != nil {
		t.Fatalf("write old backup: %v", err)
	}

	if err := app.RunMaintenance(context.Background()); err != nil {
		t.Fatalf("RunMaintenance returned error: %v", err)
	}

	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatalf("expected old backup directory to be removed, stat err=%v", err)
	}
	files, err := backup.ListFiles(app.Config.BackupDir)
	if err != nil {
		t.Fatalf("ListFiles returned error: %v", err)
	}
	if len(files) != 1 || filepath.Base(filepath.Dir(files[0])) != "2026-05-04" {
		t.Fatalf("unexpected backup files: %+v", files)
	}
}

func TestRunMaintenanceSkipsBackupWhenDisabled(t *testing.T) {
	root := t.TempDir()
	app := newTestAppWithConfig(t, Config{
		Enabled:       true,
		DatabasePath:  filepath.Join(root, "usage.db"),
		BasePath:      "/usage",
		BackupEnabled: false,
		BackupDir:     filepath.Join(root, "backups"),
		RetentionDays: 30,
	})

	if err := app.RunMaintenance(context.Background()); err != nil {
		t.Fatalf("RunMaintenance returned error: %v", err)
	}
	files, err := backup.ListFiles(app.Config.BackupDir)
	if err != nil {
		t.Fatalf("ListFiles returned error: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected no backup files when backup disabled, got %+v", files)
	}
}

func TestMaintenanceWorkerCanStopBeforeFirstTick(t *testing.T) {
	app := newTestAppWithConfig(t, Config{
		Enabled:        true,
		DatabasePath:   filepath.Join(t.TempDir(), "usage.db"),
		BasePath:       "/usage",
		BackupEnabled:  false,
		BackupInterval: time.Hour,
	})
	worker := NewMaintenanceWorker(app)
	worker.Start()

	stopped := make(chan struct{})
	go func() {
		worker.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("maintenance worker Stop blocked")
	}
}

func TestMaintenanceWorkerStopCancelsInFlightRun(t *testing.T) {
	app := newTestAppWithConfig(t, Config{
		Enabled:        true,
		DatabasePath:   filepath.Join(t.TempDir(), "usage.db"),
		BasePath:       "/usage",
		BackupEnabled:  false,
		BackupInterval: time.Millisecond,
	})
	entered := make(chan struct{})
	worker := NewMaintenanceWorker(app)
	worker.runMaintenance = func(ctx context.Context) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	}
	worker.Start()

	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("maintenance worker did not start test run")
	}

	stopped := make(chan struct{})
	go func() {
		worker.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("maintenance worker Stop did not cancel in-flight run")
	}
}
