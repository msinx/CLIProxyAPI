package management

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usagesqlite"
)

func TestGetUsageOverviewReturnsSummary(t *testing.T) {
	t.Parallel()

	router, store := newUsageTestRouter(t)
	ts := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	insertUsageTestEvent(t, store, usagesqlite.Event{
		EventKey:    "overview-1",
		Timestamp:   ts,
		Provider:    "openai",
		Model:       "gpt-5.4",
		Failed:      false,
		LatencyMS:   100,
		TotalTokens: 42,
		CreatedAt:   ts,
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/usage/overview?start=2026-05-02T00:00:00Z&end=2026-05-03T00:00:00Z", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var body usagesqlite.Overview
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Summary.RequestCount != 1 || body.Summary.TotalTokens != 42 {
		t.Fatalf("unexpected overview: %+v", body.Summary)
	}
}

func TestListUsageEventsSupportsPaginationAndFiltering(t *testing.T) {
	t.Parallel()

	router, store := newUsageTestRouter(t)
	ts := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	insertUsageTestEvent(t, store, usagesqlite.Event{EventKey: "match", RequestID: "match", Timestamp: ts, Provider: "openai", Model: "gpt-5.4", SourceHash: "src_a", Failed: true, TotalTokens: 1, CreatedAt: ts})
	insertUsageTestEvent(t, store, usagesqlite.Event{EventKey: "skip", RequestID: "skip", Timestamp: ts, Provider: "openai", Model: "claude", SourceHash: "src_b", Failed: false, TotalTokens: 2, CreatedAt: ts})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/usage/events?page=1&page_size=1&model=gpt-5.4&source_hash=src_a&result=failed&start=2026-05-02T00:00:00Z&end=2026-05-03T00:00:00Z", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var body usagesqlite.EventsPage
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.TotalCount != 1 || len(body.Events) != 1 {
		t.Fatalf("unexpected events page: %+v", body)
	}
	if body.Events[0].RequestID != "match" || !body.Events[0].Failed {
		t.Fatalf("unexpected event: %+v", body.Events[0])
	}
}

func TestListUsageCredentialsIncludesSourceAndAuthIndex(t *testing.T) {
	t.Parallel()

	router, store := newUsageTestRouter(t)
	ts := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	insertUsageTestEvent(t, store, usagesqlite.Event{
		EventKey:   "cred",
		Timestamp:  ts,
		Source:     "alice@example.com",
		SourceHash: "src_a",
		AuthIndex:  "3",
		AuthIDHash: "auth_a",
		AuthType:   "oauth",
		Failed:     true,
		CreatedAt:  ts,
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/usage/credentials?start=2026-05-02T00:00:00Z&end=2026-05-03T00:00:00Z", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var body []usagesqlite.CredentialRow
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body) != 1 {
		t.Fatalf("len(body) = %d, want 1: %+v", len(body), body)
	}
	if body[0].SourceHash != "src_a" || body[0].AuthIndex != "3" || body[0].AuthIDHash != "auth_a" || body[0].AuthType != "oauth" {
		t.Fatalf("credential row missing identity fields: %+v", body[0])
	}
	if body[0].SourceDisplay != "OAuth · a***@example.com" {
		t.Fatalf("SourceDisplay = %q, want enriched masked email", body[0].SourceDisplay)
	}
	if body[0].FailureCount != 1 {
		t.Fatalf("FailureCount = %d, want 1", body[0].FailureCount)
	}
}

func TestUsageOverviewReturnsBadRequestForInvalidTime(t *testing.T) {
	t.Parallel()

	router, _ := newUsageTestRouter(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/usage/overview?start=not-time", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%s", w.Code, w.Body.String())
	}
}

func TestUsageHandlersReturnEmptyDataWhenStoreMissing(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	handler := NewHandler(&config.Config{}, "", nil)
	router := gin.New()
	router.GET("/usage/overview", handler.GetUsageOverview)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/usage/overview", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var body usagesqlite.Overview
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Summary.RequestCount != 0 {
		t.Fatalf("RequestCount = %d, want 0", body.Summary.RequestCount)
	}
}

func newUsageTestRouter(t *testing.T) (*gin.Engine, *usagesqlite.Store) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := usagesqlite.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "usage.db"))
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	t.Cleanup(func() {
		if errClose := db.Close(); errClose != nil {
			t.Fatalf("Close() error = %v", errClose)
		}
	})
	store := usagesqlite.NewStore(db)
	handler := NewHandler(&config.Config{}, "", nil)
	handler.SetUsageStore(store)
	router := gin.New()
	router.GET("/usage/overview", handler.GetUsageOverview)
	router.GET("/usage/events", handler.ListUsageEvents)
	router.GET("/usage/analysis", handler.GetUsageAnalysis)
	router.GET("/usage/credentials", handler.ListUsageCredentials)
	router.GET("/usage/filter-options", handler.GetUsageFilterOptions)
	return router, store
}

func insertUsageTestEvent(t *testing.T, store *usagesqlite.Store, event usagesqlite.Event) {
	t.Helper()
	if err := store.InsertEvent(context.Background(), event); err != nil {
		t.Fatalf("InsertEvent(%s) error = %v", event.EventKey, err)
	}
}
