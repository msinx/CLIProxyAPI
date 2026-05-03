package usagesqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreInsertEventAndListEvents(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTestStore(t, ctx)
	ts := time.Date(2026, 5, 2, 10, 30, 0, 0, time.UTC)

	if err := store.InsertEvent(ctx, Event{
		EventKey:        "req_1",
		RequestID:       "req_1",
		Timestamp:       ts,
		Provider:        "openai",
		Model:           "gpt-5.4",
		Endpoint:        "/v1/chat/completions",
		APIGroupKey:     "key:abc123",
		Source:          "alice@example.com",
		SourceHash:      "src_hash",
		AuthIndex:       "0",
		AuthIDHash:      "auth_hash",
		AuthType:        "api-key",
		APIKeyHash:      "key_hash",
		Failed:          false,
		StatusCode:      200,
		LatencyMS:       125,
		InputTokens:     10,
		OutputTokens:    20,
		ReasoningTokens: 3,
		CachedTokens:    2,
		TotalTokens:     33,
		CreatedAt:       ts,
	}); err != nil {
		t.Fatalf("InsertEvent() error = %v", err)
	}

	page, err := store.ListEvents(ctx, QueryFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if page.TotalCount != 1 {
		t.Fatalf("TotalCount = %d, want 1", page.TotalCount)
	}
	if len(page.Events) != 1 {
		t.Fatalf("len(Events) = %d, want 1", len(page.Events))
	}
	got := page.Events[0]
	if got.RequestID != "req_1" || got.Model != "gpt-5.4" || got.TotalTokens != 33 {
		t.Fatalf("unexpected event view: %+v", got)
	}
	if got.SourceDisplay != "OpenAI · API key · a***@example.com" {
		t.Fatalf("SourceDisplay = %q, want enriched masked email", got.SourceDisplay)
	}
}

func TestStoreDoesNotDisplayShortSourceRaw(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTestStore(t, ctx)
	ts := time.Date(2026, 5, 2, 10, 30, 0, 0, time.UTC)
	if err := store.InsertEvent(ctx, Event{
		EventKey:   "short-secret",
		Timestamp:  ts,
		Source:     "sk-1234",
		SourceHash: "src_hash",
		CreatedAt:  ts,
	}); err != nil {
		t.Fatalf("InsertEvent() error = %v", err)
	}

	page, err := store.ListEvents(ctx, QueryFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if strings.Contains(page.Events[0].SourceDisplay, "sk-1234") {
		t.Fatalf("SourceDisplay exposed raw short source")
	}
	if strings.Contains(page.Sources[0].Display, "sk-1234") {
		t.Fatalf("source option exposed raw short source")
	}
}

func TestStoreResolvesCredentialDisplayWithoutExposingRawSource(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTestStore(t, ctx)
	ts := time.Date(2026, 5, 2, 10, 30, 0, 0, time.UTC)
	if err := store.InsertEvent(ctx, Event{
		EventKey:   "display",
		Timestamp:  ts,
		Provider:   "openai",
		Source:     "alice@example.com",
		SourceHash: "src_hash",
		AuthIndex:  "3",
		AuthType:   "oauth",
		CreatedAt:  ts,
	}); err != nil {
		t.Fatalf("InsertEvent() error = %v", err)
	}

	page, err := store.ListEvents(ctx, QueryFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if got, want := page.Events[0].SourceDisplay, "OpenAI · OAuth · a***@example.com"; got != want {
		t.Fatalf("SourceDisplay = %q, want %q", got, want)
	}
	if page.Events[0].SourceType != "oauth" {
		t.Fatalf("SourceType = %q, want oauth", page.Events[0].SourceType)
	}
	if page.Events[0].SourceKey != "src_hash" {
		t.Fatalf("SourceKey = %q, want source hash", page.Events[0].SourceKey)
	}
	if strings.Contains(page.Events[0].SourceDisplay, "alice@example.com") {
		t.Fatalf("SourceDisplay exposed raw source: %q", page.Events[0].SourceDisplay)
	}
	if got, want := page.Sources[0].Display, "OpenAI · OAuth · a***@example.com"; got != want {
		t.Fatalf("source option display = %q, want %q", got, want)
	}
}

func TestStoreInsertEventIgnoresDuplicateEventKey(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTestStore(t, ctx)
	ts := time.Date(2026, 5, 2, 10, 30, 0, 0, time.UTC)
	event := Event{EventKey: "same", RequestID: "same", Timestamp: ts, Model: "gpt-5.4", TotalTokens: 10, CreatedAt: ts}

	if err := store.InsertEvent(ctx, event); err != nil {
		t.Fatalf("InsertEvent() first error = %v", err)
	}
	event.TotalTokens = 99
	if err := store.InsertEvent(ctx, event); err != nil {
		t.Fatalf("InsertEvent() second error = %v", err)
	}

	overview, err := store.GetOverview(ctx, QueryFilter{})
	if err != nil {
		t.Fatalf("GetOverview() error = %v", err)
	}
	if overview.Summary.RequestCount != 1 {
		t.Fatalf("RequestCount = %d, want 1", overview.Summary.RequestCount)
	}
	if overview.Summary.TotalTokens != 10 {
		t.Fatalf("TotalTokens = %d, want first insert value 10", overview.Summary.TotalTokens)
	}
}

func TestStoreInsertEventsWritesBatchAndIgnoresDuplicates(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTestStore(t, ctx)
	ts := time.Date(2026, 5, 2, 10, 30, 0, 0, time.UTC)
	events := []Event{
		{EventKey: "batch-1", RequestID: "batch-1", Timestamp: ts, Model: "gpt-5.4", InputTokens: 10, OutputTokens: 20, CreatedAt: ts},
		{EventKey: "batch-2", RequestID: "batch-2", Timestamp: ts.Add(time.Minute), Model: "gpt-5.4", CachedTokens: 7, CreatedAt: ts},
		{EventKey: "batch-1", RequestID: "batch-1", Timestamp: ts.Add(2 * time.Minute), Model: "gpt-5.4", TotalTokens: 99, CreatedAt: ts},
	}

	if err := store.InsertEvents(ctx, events); err != nil {
		t.Fatalf("InsertEvents() error = %v", err)
	}

	overview, err := store.GetOverview(ctx, QueryFilter{})
	if err != nil {
		t.Fatalf("GetOverview() error = %v", err)
	}
	if overview.Summary.RequestCount != 2 {
		t.Fatalf("RequestCount = %d, want 2", overview.Summary.RequestCount)
	}
	if overview.Summary.TotalTokens != 37 {
		t.Fatalf("TotalTokens = %d, want normalized first values 37", overview.Summary.TotalTokens)
	}
}

func TestStoreInsertEventsRollsBackInvalidBatch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTestStore(t, ctx)
	ts := time.Date(2026, 5, 2, 10, 30, 0, 0, time.UTC)

	err := store.InsertEvents(ctx, []Event{
		{EventKey: "valid-before-error", Timestamp: ts, TotalTokens: 10, CreatedAt: ts},
		{EventKey: " ", Timestamp: ts, TotalTokens: 20, CreatedAt: ts},
	})
	if err == nil {
		t.Fatalf("InsertEvents() error = nil, want invalid event key error")
	}

	page, err := store.ListEvents(ctx, QueryFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if page.TotalCount != 0 {
		t.Fatalf("TotalCount = %d, want rollback to keep batch atomic", page.TotalCount)
	}
}

func TestStoreEmptyResultsReturnJSONArrays(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTestStore(t, ctx)
	overview, err := store.GetOverview(ctx, QueryFilter{})
	if err != nil {
		t.Fatalf("GetOverview() error = %v", err)
	}
	if overview.HourlySeries == nil || overview.DailySeries == nil || overview.Models == nil ||
		overview.Providers == nil || overview.APIKeys == nil {
		t.Fatalf("overview has nil slices: %+v", overview)
	}

	page, err := store.ListEvents(ctx, QueryFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if page.Events == nil || page.Models == nil || page.Providers == nil || page.Sources == nil {
		t.Fatalf("events page has nil slices: %+v", page)
	}

	credentials, err := store.ListCredentials(ctx, QueryFilter{})
	if err != nil {
		t.Fatalf("ListCredentials() error = %v", err)
	}
	if credentials == nil {
		t.Fatalf("credentials is nil, want empty slice")
	}
}

func TestStoreOverviewAppliesFiltersAndComputesSummary(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTestStore(t, ctx)
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{EventKey: "ok-1", Timestamp: base, Provider: "openai", Model: "gpt-5.4", SourceHash: "src_a", AuthIndex: "0", AuthIDHash: "auth_a", Failed: false, StatusCode: 200, LatencyMS: 100, InputTokens: 10, OutputTokens: 20, TotalTokens: 30, CreatedAt: base},
		{EventKey: "fail-1", Timestamp: base.Add(time.Hour), Provider: "openai", Model: "gpt-5.4", SourceHash: "src_a", AuthIndex: "0", AuthIDHash: "auth_a", Failed: true, StatusCode: 500, LatencyMS: 300, InputTokens: 5, OutputTokens: 7, ReasoningTokens: 2, CachedTokens: 1, TotalTokens: 14, CreatedAt: base},
		{EventKey: "other", Timestamp: base.Add(time.Hour), Provider: "anthropic", Model: "claude", SourceHash: "src_b", AuthIndex: "1", AuthIDHash: "auth_b", Failed: false, StatusCode: 200, LatencyMS: 1000, InputTokens: 100, OutputTokens: 100, TotalTokens: 200, CreatedAt: base},
	}
	for _, event := range events {
		if err := store.InsertEvent(ctx, event); err != nil {
			t.Fatalf("InsertEvent(%s) error = %v", event.EventKey, err)
		}
	}

	start := base.Add(-time.Minute)
	end := base.Add(2 * time.Hour)
	overview, err := store.GetOverview(ctx, QueryFilter{
		StartTime:  &start,
		EndTime:    &end,
		Model:      "gpt-5.4",
		Provider:   "openai",
		SourceHash: "src_a",
		AuthIndex:  "0",
		AuthIDHash: "auth_a",
	})
	if err != nil {
		t.Fatalf("GetOverview() error = %v", err)
	}

	if overview.Summary.RequestCount != 2 {
		t.Fatalf("RequestCount = %d, want 2", overview.Summary.RequestCount)
	}
	if overview.Summary.SuccessCount != 1 || overview.Summary.FailureCount != 1 {
		t.Fatalf("success/failure = %d/%d, want 1/1", overview.Summary.SuccessCount, overview.Summary.FailureCount)
	}
	if overview.Summary.SuccessRate != 0.5 {
		t.Fatalf("SuccessRate = %v, want 0.5", overview.Summary.SuccessRate)
	}
	if overview.Summary.TotalTokens != 44 {
		t.Fatalf("TotalTokens = %d, want 44", overview.Summary.TotalTokens)
	}
	if overview.Summary.AverageLatencyMS != 200 {
		t.Fatalf("AverageLatencyMS = %v, want 200", overview.Summary.AverageLatencyMS)
	}
	if len(overview.HourlySeries) != 2 {
		t.Fatalf("len(HourlySeries) = %d, want 2", len(overview.HourlySeries))
	}
}

func TestStoreCostAnalyticsUsesConfiguredModelPrices(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTestStore(t, ctx)
	store.SetModelPrices(map[string]ModelPrice{
		"gpt-5.4": {
			PromptPricePer1M:     1,
			CompletionPricePer1M: 10,
			CachePricePer1M:      0.5,
		},
	})
	base := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{EventKey: "priced-1", RequestID: "priced-1", Timestamp: base, Provider: "openai", Model: "gpt-5.4", InputTokens: 1_000_000, OutputTokens: 500_000, CachedTokens: 200_000, ReasoningTokens: 9_000_000, TotalTokens: 10_700_000, CreatedAt: base},
		{EventKey: "priced-2", RequestID: "priced-2", Timestamp: base.Add(time.Hour), Provider: "openai", Model: "gpt-5.4", InputTokens: 100_000, OutputTokens: 100_000, CachedTokens: 0, TotalTokens: 200_000, CreatedAt: base},
	}
	for _, event := range events {
		if err := store.InsertEvent(ctx, event); err != nil {
			t.Fatalf("InsertEvent(%s) error = %v", event.EventKey, err)
		}
	}

	overview, err := store.GetOverview(ctx, QueryFilter{})
	if err != nil {
		t.Fatalf("GetOverview() error = %v", err)
	}
	if !overview.Summary.CostAvailable {
		t.Fatalf("Summary.CostAvailable = false, want true")
	}
	if !floatEquals(overview.Summary.TotalCost, 7.2) {
		t.Fatalf("Summary.TotalCost = %v, want 7.2", overview.Summary.TotalCost)
	}
	if len(overview.Models) != 1 || !overview.Models[0].CostAvailable || !floatEquals(overview.Models[0].TotalCost, 7.2) {
		t.Fatalf("model cost row = %+v, want available total 7.2", overview.Models)
	}
	if len(overview.HourlySeries) != 2 || !floatEquals(overview.HourlySeries[0].TotalCost, 6.1) ||
		!overview.HourlySeries[0].CostAvailable || !floatEquals(overview.HourlySeries[1].TotalCost, 1.1) {
		t.Fatalf("hourly cost series = %+v, want per-bucket costs", overview.HourlySeries)
	}

	page, err := store.ListEvents(ctx, QueryFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if len(page.Events) != 2 {
		t.Fatalf("len(Events) = %d, want 2", len(page.Events))
	}
	for _, event := range page.Events {
		if !event.CostAvailable {
			t.Fatalf("event %s CostAvailable = false, want true", event.RequestID)
		}
	}
	if !floatEquals(page.Events[0].EstimatedCost, 1.1) || !floatEquals(page.Events[1].EstimatedCost, 6.1) {
		t.Fatalf("event costs = %+v, want newest 1.1 and oldest 6.1", page.Events)
	}
}

func TestStoreCostAnalyticsReportsPartialPricing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTestStore(t, ctx)
	store.SetModelPrices(map[string]ModelPrice{
		"priced": {PromptPricePer1M: 1},
	})
	ts := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	for _, event := range []Event{
		{EventKey: "priced", RequestID: "priced", Timestamp: ts, Provider: "openai", Model: "priced", InputTokens: 1_000_000, TotalTokens: 1_000_000, CreatedAt: ts},
		{EventKey: "unpriced", RequestID: "unpriced", Timestamp: ts, Provider: "openai", Model: "unpriced", InputTokens: 1_000_000, TotalTokens: 1_000_000, CreatedAt: ts},
	} {
		if err := store.InsertEvent(ctx, event); err != nil {
			t.Fatalf("InsertEvent(%s) error = %v", event.EventKey, err)
		}
	}

	overview, err := store.GetOverview(ctx, QueryFilter{})
	if err != nil {
		t.Fatalf("GetOverview() error = %v", err)
	}
	if overview.Summary.CostAvailable {
		t.Fatalf("Summary.CostAvailable = true, want false for partially priced range")
	}
	if !floatEquals(overview.Summary.TotalCost, 1) {
		t.Fatalf("Summary.TotalCost = %v, want priced subtotal 1", overview.Summary.TotalCost)
	}

	page, err := store.ListEvents(ctx, QueryFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	byRequest := map[string]EventView{}
	for _, event := range page.Events {
		byRequest[event.RequestID] = event
	}
	if !byRequest["priced"].CostAvailable || !floatEquals(byRequest["priced"].EstimatedCost, 1) {
		t.Fatalf("priced event cost = %+v, want available subtotal 1", byRequest["priced"])
	}
	if byRequest["unpriced"].CostAvailable || byRequest["unpriced"].EstimatedCost != 0 {
		t.Fatalf("unpriced event cost = %+v, want unavailable zero", byRequest["unpriced"])
	}
}

func TestStoreListCredentialsSeparatesProvidersAndResolvesDisplay(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTestStore(t, ctx)
	ts := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	for _, event := range []Event{
		{EventKey: "openai", Timestamp: ts, Provider: "openai", Source: "alice@example.com", SourceHash: "src_a", AuthIndex: "3", AuthIDHash: "auth_a", AuthType: "oauth", TotalTokens: 10, CreatedAt: ts},
		{EventKey: "claude", Timestamp: ts, Provider: "claude", Source: "alice@example.com", SourceHash: "src_a", AuthIndex: "3", AuthIDHash: "auth_a", AuthType: "oauth", TotalTokens: 20, CreatedAt: ts},
	} {
		if err := store.InsertEvent(ctx, event); err != nil {
			t.Fatalf("InsertEvent(%s) error = %v", event.EventKey, err)
		}
	}

	rows, err := store.ListCredentials(ctx, QueryFilter{})
	if err != nil {
		t.Fatalf("ListCredentials() error = %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want provider-separated rows: %+v", len(rows), rows)
	}
	got := map[string]CredentialRow{}
	for _, row := range rows {
		got[row.Provider] = row
		if strings.Contains(row.SourceDisplay, "alice@example.com") {
			t.Fatalf("SourceDisplay exposed raw source: %q", row.SourceDisplay)
		}
		if row.SourceType != "oauth" || row.SourceKey != "src_a" {
			t.Fatalf("row source metadata = type %q key %q, want oauth/src_a", row.SourceType, row.SourceKey)
		}
	}
	if got["openai"].SourceDisplay != "OpenAI · OAuth · a***@example.com" {
		t.Fatalf("openai SourceDisplay = %q", got["openai"].SourceDisplay)
	}
	if got["claude"].SourceDisplay != "Claude · OAuth · a***@example.com" {
		t.Fatalf("claude SourceDisplay = %q", got["claude"].SourceDisplay)
	}
	if got["openai"].TotalTokens != 10 || got["claude"].TotalTokens != 20 {
		t.Fatalf("provider totals not separated: %+v", got)
	}
}

func newTestStore(t *testing.T, ctx context.Context) *Store {
	t.Helper()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "usage.db"))
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	t.Cleanup(func() {
		if errClose := db.Close(); errClose != nil {
			t.Fatalf("Close() error = %v", errClose)
		}
	})
	return NewStore(db)
}

func floatEquals(got, want float64) bool {
	const epsilon = 0.0000001
	if got > want {
		return got-want < epsilon
	}
	return want-got < epsilon
}
