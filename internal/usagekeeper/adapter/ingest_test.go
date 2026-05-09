package adapter

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	internallogging "github.com/router-for-me/CLIProxyAPI/v6/internal/logging"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usagekeeper/upstream/repository"
	repodto "github.com/router-for-me/CLIProxyAPI/v6/internal/usagekeeper/upstream/repository/dto"
	servicedto "github.com/router-for-me/CLIProxyAPI/v6/internal/usagekeeper/upstream/service/dto"
	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestPluginIngestsUsageRecordsAndDedupeByRequestID(t *testing.T) {
	app := newTestApp(t)
	ctx := internallogging.WithRequestID(context.Background(), "request-1")
	record := coreusage.Record{
		Provider:    "gemini",
		Model:       "gemini-2.5-pro",
		APIKey:      "raw-secret",
		AuthIndex:   "auth-1",
		Source:      "chat",
		RequestedAt: time.Date(2026, 5, 4, 1, 2, 3, 0, time.UTC),
		Latency:     1234 * time.Millisecond,
		Detail: coreusage.Detail{
			InputTokens:     10,
			OutputTokens:    20,
			ReasoningTokens: 3,
			CachedTokens:    4,
		},
	}

	app.Plugin.HandleUsage(ctx, record)
	app.Plugin.HandleUsage(ctx, record)

	events, err := app.UsageProvider.ListUsageEvents(context.Background(), servicedto.UsageFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListUsageEvents returned error: %v", err)
	}
	if events.TotalCount != 1 {
		t.Fatalf("TotalCount = %d, want 1 after duplicate request id", events.TotalCount)
	}
	got := events.Events[0]
	if got.APIGroupKey == "raw-secret" {
		t.Fatal("raw API key was persisted in APIGroupKey")
	}
	if got.APIGroupKey == "" {
		t.Fatal("APIGroupKey is empty")
	}
	if got.TotalTokens != 33 {
		t.Fatalf("TotalTokens = %d, want computed total 33", got.TotalTokens)
	}
	if got.LatencyMS != 1234 {
		t.Fatalf("LatencyMS = %d, want 1234", got.LatencyMS)
	}
}

func TestPluginKeepsRecordsWithoutRequestIDDistinct(t *testing.T) {
	app := newTestApp(t)
	record := coreusage.Record{
		Provider:    "claude",
		Model:       "claude-sonnet",
		AuthIndex:   "auth-2",
		Source:      "responses",
		RequestedAt: time.Date(2026, 5, 4, 1, 2, 3, 0, time.UTC),
		Latency:     5 * time.Millisecond,
		Detail:      coreusage.Detail{InputTokens: 1, OutputTokens: 2, TotalTokens: 3},
	}

	app.Plugin.HandleUsage(context.Background(), record)
	app.Plugin.HandleUsage(context.Background(), record)

	events, err := app.UsageProvider.ListUsageEvents(context.Background(), servicedto.UsageFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListUsageEvents returned error: %v", err)
	}
	if events.TotalCount != 2 {
		t.Fatalf("TotalCount = %d, want 2 without request id", events.TotalCount)
	}
}

func TestPluginCloseDisablesFurtherWrites(t *testing.T) {
	app := newTestApp(t)

	app.Plugin.HandleUsage(context.Background(), coreusage.Record{
		Provider:    "gemini",
		Model:       "model",
		RequestedAt: time.Now(),
		Detail:      coreusage.Detail{TotalTokens: 1},
	})
	if err := app.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	app.Plugin.HandleUsage(context.Background(), coreusage.Record{
		Provider:    "gemini",
		Model:       "model",
		RequestedAt: time.Now(),
		Detail:      coreusage.Detail{TotalTokens: 1},
	})
}

func TestClosingOldAppDoesNotDisableNewAppStore(t *testing.T) {
	oldApp := newTestApp(t)
	newApp := newTestApp(t)

	if err := oldApp.Close(); err != nil {
		t.Fatalf("old app Close returned error: %v", err)
	}

	newApp.Plugin.HandleUsage(context.Background(), coreusage.Record{
		Provider:    "gemini",
		Model:       "model",
		RequestedAt: time.Now(),
		Detail:      coreusage.Detail{TotalTokens: 1},
	})

	events, err := newApp.UsageProvider.ListUsageEvents(context.Background(), servicedto.UsageFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListUsageEvents returned error: %v", err)
	}
	if events.TotalCount != 1 {
		t.Fatalf("TotalCount = %d, want new app to remain writable", events.TotalCount)
	}
}

func TestConcurrentIngestAndQueriesDoNotLockDatabase(t *testing.T) {
	app := newTestApp(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			app.Plugin.HandleUsage(ctx, coreusage.Record{
				Provider:    "gemini",
				Model:       "gemini-2.5-pro",
				AuthIndex:   "auth",
				Source:      "chat",
				RequestedAt: time.Date(2026, 5, 4, 1, 2, i%59, 0, time.UTC),
				Detail:      coreusage.Detail{InputTokens: 1, OutputTokens: 2, TotalTokens: 3},
			})
			if _, err := app.UsageProvider.GetUsageOverview(ctx, servicedto.UsageFilter{}); err != nil {
				t.Errorf("GetUsageOverview returned error: %v", err)
			}
		}(i)
	}
	wg.Wait()
}

func TestIngestedOverviewCountsSuccessFailureAndTokens(t *testing.T) {
	app := newTestApp(t)
	ctx := context.Background()

	app.Plugin.HandleUsage(ctx, coreusage.Record{
		Provider:    "gemini",
		Model:       "m",
		RequestedAt: time.Now(),
		Latency:     10 * time.Millisecond,
		Detail:      coreusage.Detail{InputTokens: 2, OutputTokens: 3, TotalTokens: 5},
	})
	app.Plugin.HandleUsage(ctx, coreusage.Record{
		Provider:    "gemini",
		Model:       "m",
		RequestedAt: time.Now(),
		Latency:     20 * time.Millisecond,
		Failed:      true,
		Detail:      coreusage.Detail{InputTokens: 7, OutputTokens: 11, TotalTokens: 18},
	})

	snapshot, err := repository.BuildUsageSnapshotWithFilter(app.DB, repodto.UsageQueryFilter{})
	if err != nil {
		t.Fatalf("BuildUsageSnapshotWithFilter returned error: %v", err)
	}
	if snapshot.TotalRequests != 2 {
		t.Fatalf("TotalRequests = %d, want 2", snapshot.TotalRequests)
	}
	if snapshot.SuccessCount != 1 {
		t.Fatalf("SuccessCount = %d, want 1", snapshot.SuccessCount)
	}
	if snapshot.FailureCount != 1 {
		t.Fatalf("FailureCount = %d, want 1", snapshot.FailureCount)
	}
	if snapshot.TotalTokens != 23 {
		t.Fatalf("TotalTokens = %d, want 23", snapshot.TotalTokens)
	}
	latencyPreserved := false
	for _, api := range snapshot.APIs {
		for _, model := range api.Models {
			for _, detail := range model.Details {
				if detail.LatencyMS > 0 {
					latencyPreserved = true
				}
			}
		}
	}
	if !latencyPreserved {
		t.Fatal("latency sample was not preserved")
	}
}

func newTestApp(t *testing.T) *App {
	t.Helper()

	return newTestAppWithConfig(t, Config{
		Enabled:      true,
		DatabasePath: filepath.Join(t.TempDir(), "usage.db"),
		BasePath:     "/usage",
	})
}

func newTestAppWithConfig(t *testing.T, cfg Config) *App {
	t.Helper()
	return newTestAppWithConfigAndOptions(t, cfg, Options{})
}

func newTestAppWithOptions(t *testing.T, opts Options) *App {
	t.Helper()
	return newTestAppWithConfigAndOptions(t, Config{
		Enabled:      true,
		DatabasePath: filepath.Join(t.TempDir(), "usage.db"),
		BasePath:     "/usage",
	}, opts)
}

func newTestAppWithConfigAndOptions(t *testing.T, cfg Config, opts Options) *App {
	t.Helper()

	app, err := NewAppWithOptions(cfg, opts)
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := app.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	})
	return app
}
