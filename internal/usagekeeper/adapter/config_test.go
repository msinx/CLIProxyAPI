package adapter

import (
	"path/filepath"
	"testing"
)

func TestNormalizeConfigDefaultsBasePathAndTrimsSlash(t *testing.T) {
	cfg, err := NormalizeConfig(Config{BasePath: "/usage/"}, "")
	if err != nil {
		t.Fatalf("NormalizeConfig returned error: %v", err)
	}
	if cfg.BasePath != "/usage" {
		t.Fatalf("BasePath = %q, want /usage", cfg.BasePath)
	}

	cfg, err = NormalizeConfig(Config{}, "")
	if err != nil {
		t.Fatalf("NormalizeConfig returned error: %v", err)
	}
	if cfg.BasePath != "/usage" {
		t.Fatalf("empty BasePath normalized to %q, want /usage", cfg.BasePath)
	}
}

func TestNormalizeConfigResolvesRelativePathsFromConfigDir(t *testing.T) {
	configDir := t.TempDir()

	cfg, err := NormalizeConfig(Config{
		DatabasePath: "data/usage.db",
		BackupDir:    "data/backups",
	}, configDir)
	if err != nil {
		t.Fatalf("NormalizeConfig returned error: %v", err)
	}

	wantDB := filepath.Join(configDir, "data", "usage.db")
	if cfg.DatabasePath != wantDB {
		t.Fatalf("DatabasePath = %q, want %q", cfg.DatabasePath, wantDB)
	}

	wantBackup := filepath.Join(configDir, "data", "backups")
	if cfg.BackupDir != wantBackup {
		t.Fatalf("BackupDir = %q, want %q", cfg.BackupDir, wantBackup)
	}
}

func TestNewDisabledAppDoesNotCreateDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "usage.db")

	enabled := false
	app, err := NewAppWithOptions(Config{
		DatabasePath: dbPath,
	}, Options{Enabled: &enabled})
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}
	if app.DB != nil {
		t.Fatal("disabled app opened a database")
	}
	if app.Plugin == nil {
		t.Fatal("disabled app should still expose the singleton plugin")
	}
}
