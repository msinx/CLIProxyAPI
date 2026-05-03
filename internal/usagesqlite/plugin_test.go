package usagesqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	internallogging "github.com/router-for-me/CLIProxyAPI/v6/internal/logging"
	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestPluginConvertsUsageRecordToEventAndHashesSensitiveFields(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = internallogging.WithRequestID(ctx, "req-plugin")
	ctx = internallogging.WithEndpoint(ctx, "/v1/chat/completions")
	ctx = internallogging.WithResponseStatusHolder(ctx)
	internallogging.SetResponseStatus(ctx, 201)
	store := newTestStore(t, ctx)
	plugin := NewPlugin(store, "stable-salt", PluginOptions{Enabled: true, BufferSize: 8, BatchSize: 1, FlushInterval: time.Hour})
	plugin.Start(context.Background())
	t.Cleanup(func() { stopPlugin(t, plugin) })

	plugin.HandleUsage(ctx, coreusage.Record{
		Provider:    "openai",
		Model:       "gpt-5.4",
		APIKey:      "sk-secret",
		AuthID:      "auth-secret",
		AuthIndex:   "2",
		AuthType:    "api-key",
		Source:      "alice@example.com",
		RequestedAt: time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC),
		Latency:     250 * time.Millisecond,
		Detail:      coreusage.Detail{InputTokens: 10, OutputTokens: 20, ReasoningTokens: 3, CachedTokens: 1},
	})
	waitForEvents(t, store, 1)

	page, err := store.ListEvents(ctx, QueryFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	event := page.Events[0]
	if event.RequestID != "req-plugin" {
		t.Fatalf("RequestID = %q, want req-plugin", event.RequestID)
	}
	if event.Provider != "openai" || event.Model != "gpt-5.4" {
		t.Fatalf("provider/model = %q/%q", event.Provider, event.Model)
	}
	if event.Endpoint != "/v1/chat/completions" || event.StatusCode != 201 {
		t.Fatalf("endpoint/status = %q/%d", event.Endpoint, event.StatusCode)
	}
	if event.TotalTokens != 33 {
		t.Fatalf("TotalTokens = %d, want normalized total 33", event.TotalTokens)
	}
	if event.SourceDisplay != "a***@example.com" {
		t.Fatalf("SourceDisplay = %q, want masked email", event.SourceDisplay)
	}

	var apiKeyHash, sourceHash, authIDHash, apiGroupKey string
	row := store.db.sqlDB.QueryRowContext(ctx, `SELECT api_key_hash, source_hash, auth_id_hash, api_group_key FROM usage_events WHERE request_id = ?`, "req-plugin")
	if errScan := row.Scan(&apiKeyHash, &sourceHash, &authIDHash, &apiGroupKey); errScan != nil {
		t.Fatalf("scan hashes: %v", errScan)
	}
	for _, got := range []string{apiKeyHash, sourceHash, authIDHash, apiGroupKey} {
		if strings.Contains(got, "secret") || strings.Contains(got, "alice@example.com") {
			t.Fatalf("sensitive value leaked into hash/group field: %q", got)
		}
	}
	if apiKeyHash == "" || sourceHash == "" || authIDHash == "" || !strings.HasPrefix(apiGroupKey, "key:") {
		t.Fatalf("unexpected hash/group values: api=%q source=%q auth=%q group=%q", apiKeyHash, sourceHash, authIDHash, apiGroupKey)
	}
}

func TestPluginIncludesCachedTokensWhenNormalizingTotalTokens(t *testing.T) {
	t.Parallel()

	ctx := internallogging.WithRequestID(context.Background(), "req-cached-only")
	store := newTestStore(t, ctx)
	plugin := NewPlugin(store, "stable-salt", PluginOptions{Enabled: true, BufferSize: 8, BatchSize: 1, FlushInterval: time.Hour})
	plugin.Start(context.Background())
	t.Cleanup(func() { stopPlugin(t, plugin) })

	plugin.HandleUsage(ctx, coreusage.Record{
		Model:       "gpt-5.4",
		RequestedAt: time.Now(),
		Detail:      coreusage.Detail{CachedTokens: 7},
	})
	waitForEvents(t, store, 1)

	page, err := store.ListEvents(ctx, QueryFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if page.Events[0].TotalTokens != 7 {
		t.Fatalf("TotalTokens = %d, want cached-only total 7", page.Events[0].TotalTokens)
	}
}

func TestPluginKeepsMultipleRecordsForSameRequestID(t *testing.T) {
	t.Parallel()

	ctx := internallogging.WithRequestID(context.Background(), "req-multi-model")
	store := newTestStore(t, ctx)
	plugin := NewPlugin(store, "stable-salt", PluginOptions{Enabled: true, BufferSize: 8, BatchSize: 1, FlushInterval: time.Hour})
	plugin.Start(context.Background())
	t.Cleanup(func() { stopPlugin(t, plugin) })

	plugin.HandleUsage(ctx, coreusage.Record{
		Model:       "gpt-5.4",
		RequestedAt: time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC),
		Detail:      coreusage.Detail{TotalTokens: 11},
	})
	plugin.HandleUsage(ctx, coreusage.Record{
		Model:       "gpt-image-2",
		RequestedAt: time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC),
		Detail:      coreusage.Detail{TotalTokens: 7},
	})

	waitForEvents(t, store, 2)
}

func TestPluginDisabledDoesNotWriteEvents(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTestStore(t, ctx)
	plugin := NewPlugin(store, "stable-salt", PluginOptions{Enabled: false, BufferSize: 8, BatchSize: 1, FlushInterval: time.Millisecond})
	plugin.Start(context.Background())
	t.Cleanup(func() { stopPlugin(t, plugin) })

	plugin.HandleUsage(ctx, coreusage.Record{Model: "gpt-5.4", RequestedAt: time.Now()})
	time.Sleep(20 * time.Millisecond)

	page, err := store.ListEvents(ctx, QueryFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if page.TotalCount != 0 {
		t.Fatalf("TotalCount = %d, want 0", page.TotalCount)
	}
}

func TestPluginDropsWhenQueueIsFullWithoutBlocking(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTestStore(t, ctx)
	plugin := NewPlugin(store, "stable-salt", PluginOptions{Enabled: true, BufferSize: 1, BatchSize: 100, FlushInterval: time.Hour})

	start := time.Now()
	for i := 0; i < 20; i++ {
		plugin.HandleUsage(ctx, coreusage.Record{Model: "gpt-5.4", RequestedAt: time.Now()})
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("HandleUsage blocked for %v", elapsed)
	}
	if plugin.DroppedCount() == 0 {
		t.Fatalf("DroppedCount() = 0, want drops when queue is full")
	}
}

func TestPluginFlushesQueuedEventAfterRequestContextCanceled(t *testing.T) {
	t.Parallel()

	requestCtx, cancel := context.WithCancel(context.Background())
	requestCtx = internallogging.WithRequestID(requestCtx, "req-canceled")
	store := newTestStore(t, context.Background())
	plugin := NewPlugin(store, "stable-salt", PluginOptions{Enabled: true, BufferSize: 8, BatchSize: 10, FlushInterval: 10 * time.Millisecond})
	plugin.Start(context.Background())
	t.Cleanup(func() { stopPlugin(t, plugin) })

	plugin.HandleUsage(requestCtx, coreusage.Record{Model: "gpt-5.4", RequestedAt: time.Now(), Detail: coreusage.Detail{TotalTokens: 1}})
	cancel()

	waitForEvents(t, store, 1)
}

func waitForEvents(t *testing.T, store *Store, want int64) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		page, err := store.ListEvents(context.Background(), QueryFilter{Page: 1, PageSize: 10})
		if err == nil && page.TotalCount == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	page, err := store.ListEvents(context.Background(), QueryFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	t.Fatalf("TotalCount = %d, want %d", page.TotalCount, want)
}

func stopPlugin(t *testing.T, plugin *Plugin) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := plugin.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}
