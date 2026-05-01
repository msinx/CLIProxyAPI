package management

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	internalusage "github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestGetUsageStatisticsEnrichesAPIKeyDisplayMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte(`api-keys:
  - api-key: real-api-key
    alias: Team Alias
    name: Team Name
    comment: Used by tests
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	prevStatsEnabled := internalusage.StatisticsEnabled()
	internalusage.SetStatisticsEnabled(true)
	defer internalusage.SetStatisticsEnabled(prevStatsEnabled)

	stats := internalusage.NewRequestStatistics()
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "real-api-key",
		Model:       "gpt-5.4",
		RequestedAt: time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		Detail:      coreusage.Detail{InputTokens: 1, OutputTokens: 2, TotalTokens: 3},
	})

	h := NewHandlerWithoutConfigFilePath(cfg, nil)
	h.SetUsageStatistics(stats)
	r := gin.New()
	r.GET("/usage", h.GetUsageStatistics)

	req := httptest.NewRequest(http.MethodGet, "/usage", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	usageMap := body["usage"].(map[string]any)
	apis := usageMap["apis"].(map[string]any)
	api := apis["real-api-key"].(map[string]any)
	if got := api["display_name"]; got != "Team Alias (real****-key)" {
		t.Fatalf("display_name = %#v", got)
	}
	if got := api["masked_key"]; got != "real****-key" {
		t.Fatalf("masked_key = %#v", got)
	}
	if got := api["alias"]; got != "Team Alias" {
		t.Fatalf("alias = %#v", got)
	}
	if got := api["name"]; got != "Team Name" {
		t.Fatalf("name = %#v", got)
	}
	if got := api["comment"]; got != "Used by tests" {
		t.Fatalf("comment = %#v", got)
	}
}

func TestGetUsageStatisticsMasksRawAPIKeyInDisplayMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte(`api-keys:
  - api-key: real-api-key
    alias: real-api-key
    name: owner real-api-key
    comment: comment real-api-key tail
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	prevStatsEnabled := internalusage.StatisticsEnabled()
	internalusage.SetStatisticsEnabled(true)
	defer internalusage.SetStatisticsEnabled(prevStatsEnabled)

	stats := internalusage.NewRequestStatistics()
	stats.Record(context.Background(), coreusage.Record{
		APIKey:      "real-api-key",
		Model:       "gpt-5.4",
		RequestedAt: time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		Detail:      coreusage.Detail{InputTokens: 1, OutputTokens: 2, TotalTokens: 3},
	})

	h := NewHandlerWithoutConfigFilePath(cfg, nil)
	h.SetUsageStatistics(stats)
	r := gin.New()
	r.GET("/usage", h.GetUsageStatistics)

	req := httptest.NewRequest(http.MethodGet, "/usage", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	usageMap := body["usage"].(map[string]any)
	apis := usageMap["apis"].(map[string]any)
	api := apis["real-api-key"].(map[string]any)
	if got := api["display_name"]; got != "real****-key (real****-key)" {
		t.Fatalf("display_name = %#v", got)
	}
	if got := api["alias"]; got != "real****-key" {
		t.Fatalf("alias = %#v", got)
	}
	if got := api["name"]; got != "owner real****-key" {
		t.Fatalf("name = %#v", got)
	}
	if got := api["comment"]; got != "comment real****-key tail" {
		t.Fatalf("comment = %#v", got)
	}
}
