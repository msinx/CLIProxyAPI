package api

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func TestUpdateClientsCanEnableSQLiteUsageAfterStartupDisabled(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "usage.db")
	cfg := &config.Config{
		UsageStatisticsEnabled:          true,
		UsageSQLiteEnabled:              false,
		UsageSQLitePath:                 dbPath,
		UsageSQLiteBufferSize:           4,
		UsageSQLiteBatchSize:            1,
		UsageSQLiteFlushInterval:        1,
		RedisUsageQueueRetentionSeconds: 60,
	}
	server := NewServer(cfg, nil, nil, filepath.Join(t.TempDir(), "config.yaml"))
	if server.usageSQLitePlugin != nil {
		t.Fatalf("usageSQLitePlugin initialized while usage-sqlite-enabled=false")
	}

	next := *cfg
	next.UsageSQLiteEnabled = true
	server.UpdateClients(&next)

	if server.usageSQLitePlugin == nil || server.usageSQLiteStore == nil {
		t.Fatalf("sqlite usage was not initialized after enabling")
	}
}

func TestNewServerResolvesRelativeSQLiteUsagePathFromConfigDirectory(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	cfg := &config.Config{
		UsageStatisticsEnabled:          true,
		UsageSQLiteEnabled:              true,
		UsageSQLitePath:                 "data/usage.db",
		UsageSQLiteBufferSize:           4,
		UsageSQLiteBatchSize:            1,
		RedisUsageQueueRetentionSeconds: 60,
	}
	server := NewServer(cfg, nil, nil, filepath.Join(configDir, "config.yaml"))
	t.Cleanup(func() {
		if server.usageSQLiteDB != nil {
			_ = server.usageSQLiteDB.Close()
		}
	})

	wantPath := filepath.Join(configDir, "data", "usage.db")
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("expected sqlite database at %s: %v", wantPath, err)
	}
}
