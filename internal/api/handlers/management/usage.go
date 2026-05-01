package management

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
)

type usageExportPayload struct {
	Version    int                      `json:"version"`
	ExportedAt time.Time                `json:"exported_at"`
	Usage      usage.StatisticsSnapshot `json:"usage"`
}

type usageImportPayload struct {
	Version int                      `json:"version"`
	Usage   usage.StatisticsSnapshot `json:"usage"`
}

// GetUsageStatistics returns the in-memory request statistics snapshot.
func (h *Handler) GetUsageStatistics(c *gin.Context) {
	var snapshot usage.StatisticsSnapshot
	if h != nil && h.usageStats != nil {
		snapshot = h.usageStats.Snapshot()
	}
	c.JSON(http.StatusOK, gin.H{
		"usage":           enrichUsageSnapshotForDisplay(snapshot, h.configForUsageDisplay()),
		"failed_requests": snapshot.FailureCount,
	})
}

func (h *Handler) configForUsageDisplay() *config.Config {
	if h == nil {
		return nil
	}
	return h.cfg
}

func enrichUsageSnapshotForDisplay(snapshot usage.StatisticsSnapshot, cfg *config.Config) gin.H {
	apis := make(map[string]any, len(snapshot.APIs))
	for apiName, apiSnapshot := range snapshot.APIs {
		apiMap := gin.H{
			"total_requests": apiSnapshot.TotalRequests,
			"total_tokens":   apiSnapshot.TotalTokens,
			"models":         apiSnapshot.Models,
		}
		if cfg != nil && cfg.SDKConfig.HasAPIKey(apiName) {
			maskedKey := config.MaskAPIKey(apiName)
			apiMap["masked_key"] = maskedKey
			apiMap["display_name"] = cfg.SDKConfig.DisplayLabelForAPIKey(apiName)
			if metadata, ok := cfg.SDKConfig.APIKeyMetadata(apiName); ok {
				if metadata.Alias != "" {
					apiMap["alias"] = metadata.Alias
				}
				if metadata.Name != "" {
					apiMap["name"] = metadata.Name
				}
				if metadata.Comment != "" {
					apiMap["comment"] = metadata.Comment
				}
			}
		}
		apis[apiName] = apiMap
	}
	return gin.H{
		"total_requests":   snapshot.TotalRequests,
		"success_count":    snapshot.SuccessCount,
		"failure_count":    snapshot.FailureCount,
		"total_tokens":     snapshot.TotalTokens,
		"apis":             apis,
		"requests_by_day":  snapshot.RequestsByDay,
		"requests_by_hour": snapshot.RequestsByHour,
		"tokens_by_day":    snapshot.TokensByDay,
		"tokens_by_hour":   snapshot.TokensByHour,
	}
}

// ExportUsageStatistics returns a complete usage snapshot for backup/migration.
func (h *Handler) ExportUsageStatistics(c *gin.Context) {
	var snapshot usage.StatisticsSnapshot
	if h != nil && h.usageStats != nil {
		snapshot = h.usageStats.Snapshot()
	}
	c.JSON(http.StatusOK, usageExportPayload{
		Version:    1,
		ExportedAt: time.Now().UTC(),
		Usage:      snapshot,
	})
}

// ImportUsageStatistics merges a previously exported usage snapshot into memory.
func (h *Handler) ImportUsageStatistics(c *gin.Context) {
	if h == nil || h.usageStats == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "usage statistics unavailable"})
		return
	}

	data, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	var payload usageImportPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	if payload.Version != 0 && payload.Version != 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported version"})
		return
	}

	result := h.usageStats.MergeSnapshot(payload.Usage)
	snapshot := h.usageStats.Snapshot()
	c.JSON(http.StatusOK, gin.H{
		"added":           result.Added,
		"skipped":         result.Skipped,
		"total_requests":  snapshot.TotalRequests,
		"failed_requests": snapshot.FailureCount,
	})
}
