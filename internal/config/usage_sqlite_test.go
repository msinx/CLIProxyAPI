package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfigOptionalUsageSQLiteDefaults(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: 8317\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := LoadConfigOptional(configPath, false)
	if err != nil {
		t.Fatalf("LoadConfigOptional() error = %v", err)
	}
	if !cfg.UsageStatisticsEnabled {
		t.Fatalf("UsageStatisticsEnabled = false, want true")
	}
	if !cfg.UsageSQLiteEnabled {
		t.Fatalf("UsageSQLiteEnabled = false, want true")
	}
	if cfg.UsageSQLitePath != "./data/usage.db" {
		t.Fatalf("UsageSQLitePath = %q", cfg.UsageSQLitePath)
	}
	if cfg.UsageRetentionDays != 30 {
		t.Fatalf("UsageRetentionDays = %d, want 30", cfg.UsageRetentionDays)
	}
	if cfg.UsageSQLiteBufferSize != 4096 || cfg.UsageSQLiteBatchSize != 100 {
		t.Fatalf("buffer/batch = %d/%d", cfg.UsageSQLiteBufferSize, cfg.UsageSQLiteBatchSize)
	}
	if cfg.UsageSQLiteFlushInterval != time.Second {
		t.Fatalf("UsageSQLiteFlushInterval = %v, want 1s", cfg.UsageSQLiteFlushInterval)
	}
	if cfg.UsageSQLiteMaintenanceInterval != 24*time.Hour {
		t.Fatalf("UsageSQLiteMaintenanceInterval = %v, want 24h", cfg.UsageSQLiteMaintenanceInterval)
	}
	if cfg.UsageSQLiteBackupEnabled {
		t.Fatalf("UsageSQLiteBackupEnabled = true, want false")
	}
	if cfg.UsageSQLiteBackupPath != "./data/usage-backups" {
		t.Fatalf("UsageSQLiteBackupPath = %q", cfg.UsageSQLiteBackupPath)
	}
	if cfg.UsageSQLiteBackupRetentionDays != 7 {
		t.Fatalf("UsageSQLiteBackupRetentionDays = %d, want 7", cfg.UsageSQLiteBackupRetentionDays)
	}
	if len(cfg.UsageModelPrices) != 0 {
		t.Fatalf("UsageModelPrices = %+v, want empty default", cfg.UsageModelPrices)
	}
	if cfg.RemoteManagement.PanelGitHubRepository != DefaultPanelGitHubRepository {
		t.Fatalf("PanelGitHubRepository = %q", cfg.RemoteManagement.PanelGitHubRepository)
	}
}

func TestLoadConfigOptionalUsageSQLiteSanitizesInvalidValues(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte(`port: 8317
usage-sqlite-buffer-size: -1
usage-sqlite-batch-size: 0
usage-sqlite-flush-interval: 0s
usage-sqlite-maintenance-interval: 0s
usage-sqlite-backup-path: "  "
usage-sqlite-backup-retention-days: -3
usage-retention-days: -5
usage-model-prices:
  "  gpt-5.4  ":
    prompt-price-per-1m: -1
    completion-price-per-1m: 10
    cache-price-per-1m: -0.25
  "   ":
    prompt-price-per-1m: 2
`)
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := LoadConfigOptional(configPath, false)
	if err != nil {
		t.Fatalf("LoadConfigOptional() error = %v", err)
	}
	if cfg.UsageSQLiteBufferSize != 4096 {
		t.Fatalf("UsageSQLiteBufferSize = %d, want default", cfg.UsageSQLiteBufferSize)
	}
	if cfg.UsageSQLiteBatchSize != 100 {
		t.Fatalf("UsageSQLiteBatchSize = %d, want default", cfg.UsageSQLiteBatchSize)
	}
	if cfg.UsageSQLiteFlushInterval != time.Second {
		t.Fatalf("UsageSQLiteFlushInterval = %v, want default", cfg.UsageSQLiteFlushInterval)
	}
	if cfg.UsageRetentionDays != 0 {
		t.Fatalf("UsageRetentionDays = %d, want 0 for invalid negative", cfg.UsageRetentionDays)
	}
	if cfg.UsageSQLiteMaintenanceInterval != 24*time.Hour {
		t.Fatalf("UsageSQLiteMaintenanceInterval = %v, want default", cfg.UsageSQLiteMaintenanceInterval)
	}
	if cfg.UsageSQLiteBackupPath != "./data/usage-backups" {
		t.Fatalf("UsageSQLiteBackupPath = %q, want default", cfg.UsageSQLiteBackupPath)
	}
	if cfg.UsageSQLiteBackupRetentionDays != 0 {
		t.Fatalf("UsageSQLiteBackupRetentionDays = %d, want 0 for invalid negative", cfg.UsageSQLiteBackupRetentionDays)
	}
	if len(cfg.UsageModelPrices) != 1 {
		t.Fatalf("len(UsageModelPrices) = %d, want 1: %+v", len(cfg.UsageModelPrices), cfg.UsageModelPrices)
	}
	price := cfg.UsageModelPrices["gpt-5.4"]
	if price.PromptPricePer1M != 0 || price.CompletionPricePer1M != 10 || price.CachePricePer1M != 0 {
		t.Fatalf("UsageModelPrices[gpt-5.4] = %+v, want negative rates clamped and key trimmed", price)
	}
}
