package adapter

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultBasePath            = "/usage"
	defaultDatabasePath        = "./data/usage-keeper.db"
	defaultBackupDir           = "./data/usage-keeper-backups"
	defaultBackupInterval      = 24 * time.Hour
	defaultBackupRetentionDays = 7
	defaultRetentionDays       = 30
	defaultSessionTTL          = 12 * time.Hour
	defaultTimezone            = "Local"
)

type Config struct {
	Enabled             bool
	DatabasePath        string
	BasePath            string
	StaticDir           string
	BackupEnabled       bool
	BackupDir           string
	BackupInterval      time.Duration
	BackupRetentionDays int
	RetentionDays       int
	Timezone            string
	SessionTTL          time.Duration
}

func DefaultConfig() Config {
	return Config{
		Enabled:             true,
		DatabasePath:        defaultDatabasePath,
		BasePath:            defaultBasePath,
		BackupEnabled:       true,
		BackupDir:           defaultBackupDir,
		BackupInterval:      defaultBackupInterval,
		BackupRetentionDays: defaultBackupRetentionDays,
		RetentionDays:       defaultRetentionDays,
		Timezone:            defaultTimezone,
		SessionTTL:          defaultSessionTTL,
	}
}

func NormalizeConfig(input Config, configDir string) (Config, error) {
	defaults := DefaultConfig()
	cfg := input

	if strings.TrimSpace(cfg.DatabasePath) == "" {
		cfg.DatabasePath = defaults.DatabasePath
	}
	if strings.TrimSpace(cfg.BasePath) == "" {
		cfg.BasePath = defaults.BasePath
	}
	basePath, err := normalizeBasePath(cfg.BasePath)
	if err != nil {
		return Config{}, err
	}
	cfg.BasePath = basePath

	if strings.TrimSpace(cfg.BackupDir) == "" {
		cfg.BackupDir = defaults.BackupDir
	}
	if cfg.BackupInterval == 0 {
		cfg.BackupInterval = defaults.BackupInterval
	}
	if cfg.BackupRetentionDays == 0 {
		cfg.BackupRetentionDays = defaults.BackupRetentionDays
	}
	if cfg.RetentionDays == 0 {
		cfg.RetentionDays = defaults.RetentionDays
	}
	if strings.TrimSpace(cfg.Timezone) == "" {
		cfg.Timezone = defaults.Timezone
	}
	if cfg.SessionTTL == 0 {
		cfg.SessionTTL = defaults.SessionTTL
	}

	cfg.DatabasePath = resolvePath(configDir, cfg.DatabasePath)
	cfg.BackupDir = resolvePath(configDir, cfg.BackupDir)
	return cfg, nil
}

func normalizeBasePath(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return defaultBasePath, nil
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	cleaned := path.Clean(trimmed)
	if cleaned == "." || cleaned == "/" {
		return "", fmt.Errorf("usage keeper base path must not be root")
	}
	return cleaned, nil
}

func resolvePath(configDir, value string) string {
	cleaned := filepath.Clean(strings.TrimSpace(value))
	if cleaned == "" || filepath.IsAbs(cleaned) || strings.TrimSpace(configDir) == "" {
		return cleaned
	}
	return filepath.Join(configDir, cleaned)
}
