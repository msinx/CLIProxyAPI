package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUsageStatisticsEnabledDefaultsTrue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("port: 8317\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if !cfg.UsageStatisticsEnabled {
		t.Fatal("UsageStatisticsEnabled default = false, want true")
	}
}

func TestUsageStatisticsEnabledCanBeDisabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("usage-statistics-enabled: false\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if cfg.UsageStatisticsEnabled {
		t.Fatal("UsageStatisticsEnabled = true, want explicit false to be preserved")
	}
}
