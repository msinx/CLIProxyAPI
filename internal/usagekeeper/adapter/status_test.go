package adapter

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"
)

type metadataRefresherFunc func(context.Context, *gorm.DB) error

func (f metadataRefresherFunc) RefreshUsageKeeperMetadata(ctx context.Context, db *gorm.DB) error {
	return f(ctx, db)
}

func TestEmbeddedStatusAndSyncAreLocal(t *testing.T) {
	app := newTestApp(t)

	status := app.Status()
	if !status.Running {
		t.Fatal("Status().Running = false, want true")
	}
	if status.SyncRunning {
		t.Fatal("Status().SyncRunning = true, want false")
	}
	if status.LastStatus != "completed" {
		t.Fatalf("LastStatus = %q, want completed", status.LastStatus)
	}

	if err := app.SyncNow(context.Background()); err != nil {
		t.Fatalf("SyncNow returned error: %v", err)
	}
	status = app.Status()
	if status.LastStatus != "completed" {
		t.Fatalf("LastStatus after SyncNow = %q, want completed", status.LastStatus)
	}
}

func TestSyncNowRefreshesInjectedMetadata(t *testing.T) {
	calls := 0
	app := newTestAppWithOptions(t, Options{
		MetadataRefresher: metadataRefresherFunc(func(_ context.Context, db *gorm.DB) error {
			calls++
			if db == nil {
				t.Fatal("metadata refresher received nil db")
			}
			return nil
		}),
	})

	if err := app.SyncNow(context.Background()); err != nil {
		t.Fatalf("SyncNow returned error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("metadata refresher calls = %d, want 1", calls)
	}
}

func TestSyncNowMarksWarningOnMetadataRefreshFailure(t *testing.T) {
	app := newTestAppWithOptions(t, Options{
		MetadataRefresher: metadataRefresherFunc(func(context.Context, *gorm.DB) error {
			return errors.New("metadata unavailable")
		}),
	})

	if err := app.SyncNow(context.Background()); err == nil {
		t.Fatal("expected SyncNow to return metadata refresh error")
	}
	status := app.Status()
	if status.LastStatus != "completed_with_warnings" || status.LastWarning != "metadata unavailable" {
		t.Fatalf("unexpected status after metadata failure: %+v", status)
	}
}
