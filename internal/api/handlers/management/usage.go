package management

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usagesqlite"
)

func (h *Handler) GetUsageOverview(c *gin.Context) {
	filter, ok := h.parseUsageFilter(c)
	if !ok {
		return
	}
	store := h.currentUsageStore()
	if store == nil {
		c.JSON(http.StatusOK, emptyUsageOverview())
		return
	}
	overview, err := store.GetOverview(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "usage_overview_failed", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, overview)
}

func (h *Handler) GetUsageAnalysis(c *gin.Context) {
	filter, ok := h.parseUsageFilter(c)
	if !ok {
		return
	}
	store := h.currentUsageStore()
	if store == nil {
		c.JSON(http.StatusOK, gin.H{"providers": []usagesqlite.BreakdownRow{}, "models": []usagesqlite.BreakdownRow{}})
		return
	}
	providers, models, err := store.GetAnalysis(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "usage_analysis_failed", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"providers": providers, "models": models})
}

func (h *Handler) ListUsageEvents(c *gin.Context) {
	filter, ok := h.parseUsageFilter(c)
	if !ok {
		return
	}
	store := h.currentUsageStore()
	if store == nil {
		c.JSON(http.StatusOK, usagesqlite.EventsPage{
			Events:     []usagesqlite.EventView{},
			Page:       normalizePositiveInt(c.Query("page"), 1),
			PageSize:   normalizePositiveInt(c.Query("page_size"), 50),
			Models:     []string{},
			Providers:  []string{},
			Sources:    []usagesqlite.SourceOption{},
			TotalPages: 0,
		})
		return
	}
	page, err := store.ListEvents(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "usage_events_failed", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, page)
}

func (h *Handler) ListUsageCredentials(c *gin.Context) {
	filter, ok := h.parseUsageFilter(c)
	if !ok {
		return
	}
	store := h.currentUsageStore()
	if store == nil {
		c.JSON(http.StatusOK, []usagesqlite.CredentialRow{})
		return
	}
	rows, err := store.ListCredentials(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "usage_credentials_failed", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rows)
}

func (h *Handler) GetUsageFilterOptions(c *gin.Context) {
	filter, ok := h.parseUsageFilter(c)
	if !ok {
		return
	}
	store := h.currentUsageStore()
	if store == nil {
		c.JSON(http.StatusOK, gin.H{"models": []string{}, "providers": []string{}, "sources": []usagesqlite.SourceOption{}})
		return
	}
	page, err := store.ListEvents(c.Request.Context(), usagesqlite.QueryFilter{
		StartTime:  filter.StartTime,
		EndTime:    filter.EndTime,
		Page:       1,
		PageSize:   1,
		Model:      filter.Model,
		Provider:   filter.Provider,
		SourceHash: filter.SourceHash,
		AuthIndex:  filter.AuthIndex,
		AuthIDHash: filter.AuthIDHash,
		Result:     filter.Result,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "usage_filter_options_failed", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"models": page.Models, "providers": page.Providers, "sources": page.Sources})
}

func (h *Handler) currentUsageStore() *usagesqlite.Store {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.usageStore
}

func emptyUsageOverview() usagesqlite.Overview {
	return usagesqlite.Overview{
		HourlySeries: []usagesqlite.TimeBucket{},
		DailySeries:  []usagesqlite.TimeBucket{},
		Models:       []usagesqlite.BreakdownRow{},
		Providers:    []usagesqlite.BreakdownRow{},
		APIKeys:      []usagesqlite.BreakdownRow{},
		Timezone:     time.Local.String(),
	}
}

func (h *Handler) parseUsageFilter(c *gin.Context) (usagesqlite.QueryFilter, bool) {
	filter := usagesqlite.QueryFilter{
		Range:      strings.TrimSpace(c.Query("range")),
		Model:      strings.TrimSpace(c.Query("model")),
		Provider:   strings.TrimSpace(c.Query("provider")),
		SourceHash: strings.TrimSpace(c.Query("source_hash")),
		AuthIndex:  strings.TrimSpace(c.Query("auth_index")),
		AuthIDHash: strings.TrimSpace(c.Query("auth_id_hash")),
		Result:     strings.TrimSpace(c.Query("result")),
		Page:       normalizePositiveInt(c.Query("page"), 1),
		PageSize:   normalizePositiveInt(c.Query("page_size"), 50),
		Limit:      normalizePositiveInt(c.Query("limit"), 100),
	}
	if filter.Result != "" && filter.Result != "success" && filter.Result != "failed" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_result"})
		return filter, false
	}

	startRaw := strings.TrimSpace(c.Query("start"))
	endRaw := strings.TrimSpace(c.Query("end"))
	if startRaw != "" {
		start, err := time.Parse(time.RFC3339, startRaw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_start"})
			return filter, false
		}
		filter.StartTime = &start
	}
	if endRaw != "" {
		end, err := time.Parse(time.RFC3339, endRaw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_end"})
			return filter, false
		}
		filter.EndTime = &end
	}
	if filter.StartTime == nil && filter.EndTime == nil {
		start, end, ok := rangeWindow(filter.Range, time.Now())
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_range"})
			return filter, false
		}
		filter.StartTime = &start
		filter.EndTime = &end
	}
	return filter, true
}

func rangeWindow(value string, now time.Time) (time.Time, time.Time, bool) {
	end := now
	switch strings.TrimSpace(value) {
	case "", "24h":
		return end.Add(-24 * time.Hour), end, true
	case "1h":
		return end.Add(-time.Hour), end, true
	case "7d":
		return end.AddDate(0, 0, -7), end, true
	case "30d":
		return end.AddDate(0, 0, -30), end, true
	default:
		return time.Time{}, time.Time{}, false
	}
}

func normalizePositiveInt(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
