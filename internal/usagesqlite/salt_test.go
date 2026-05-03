package usagesqlite

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func TestResolveSaltUsesExplicitConfigValue(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{UsageAPIKeySalt: "explicit-salt", RemoteManagement: config.RemoteManagement{SecretKey: "management-secret"}}
	got, err := ResolveSalt(cfg, filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("ResolveSalt() error = %v", err)
	}
	if got != "explicit-salt" {
		t.Fatalf("salt = %q, want explicit", got)
	}
}

func TestResolveSaltFallsBackToManagementSecret(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{RemoteManagement: config.RemoteManagement{SecretKey: "management-secret"}}
	got, err := ResolveSalt(cfg, filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("ResolveSalt() error = %v", err)
	}
	if got != "management-secret" {
		t.Fatalf("salt = %q, want management secret", got)
	}
}

func TestResolveSaltPersistsGeneratedSalt(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := &config.Config{AuthDir: filepath.Join(dir, "auths")}
	configPath := filepath.Join(dir, "config.yaml")

	first, err := ResolveSalt(cfg, configPath)
	if err != nil {
		t.Fatalf("ResolveSalt() first error = %v", err)
	}
	second, err := ResolveSalt(cfg, configPath)
	if err != nil {
		t.Fatalf("ResolveSalt() second error = %v", err)
	}
	if first == "" || first != second {
		t.Fatalf("generated salt not stable: first=%q second=%q", first, second)
	}
	info, err := os.Stat(filepath.Join(cfg.AuthDir, "usage_api_key_salt"))
	if err != nil {
		t.Fatalf("stat salt file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("salt file perms = %v, want 0600", info.Mode().Perm())
	}
}
