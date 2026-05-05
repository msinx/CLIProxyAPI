package repository

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usagekeeper/upstream/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usagekeeper/upstream/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type StorageCleanupResult struct {
	RedisInbox RedisUsageInboxCleanupResult
}

func OpenDatabase(cfg config.Config) (*gorm.DB, error) {
	dsn := sqliteDSN(cfg.SQLitePath)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open sqlite database %s: %w", filepath.Clean(cfg.SQLitePath), err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("configure sqlite database: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
		return nil, fmt.Errorf("enable sqlite WAL: %w", err)
	}
	if err := db.Exec("PRAGMA busy_timeout=5000").Error; err != nil {
		return nil, fmt.Errorf("set sqlite busy timeout: %w", err)
	}
	if err := db.Exec("PRAGMA foreign_keys=ON").Error; err != nil {
		return nil, fmt.Errorf("enable sqlite foreign keys: %w", err)
	}

	if err := runSchemaMigrations(db); err != nil {
		return nil, fmt.Errorf("run schema migrations: %w", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		return nil, fmt.Errorf("auto migrate database: %w", err)
	}

	return db, nil
}

func sqliteDSN(path string) string {
	trimmed := strings.TrimSpace(path)
	if strings.Contains(trimmed, "?") {
		return trimmed
	}
	return trimmed + "?_busy_timeout=5000&_foreign_keys=on"
}

func InsertUsageEvents(db *gorm.DB, events []models.UsageEvent) (int, int, error) {
	if db == nil {
		return 0, 0, fmt.Errorf("database is nil")
	}
	if len(events) == 0 {
		return 0, 0, nil
	}

	const batchSize = 100
	inserted := 0

	for start := 0; start < len(events); start += batchSize {
		end := min(start+batchSize, len(events))
		batch := events[start:end]
		result := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "event_key"}},
			DoNothing: true,
		}).Create(&batch)
		if result.Error != nil {
			return 0, 0, fmt.Errorf("insert usage events: %w", result.Error)
		}
		inserted += int(result.RowsAffected)
	}

	deduped := len(events) - inserted
	return inserted, deduped, nil
}

// CleanupStorage is the daily maintenance entry point: clean Redis inbox rows,
// then run VACUUM. VACUUM must run after deletes as a separate step.
func CleanupStorage(db *gorm.DB, now time.Time) (StorageCleanupResult, error) {
	redisResult, err := CleanupRedisUsageInbox(db, now)
	if err != nil {
		return StorageCleanupResult{RedisInbox: redisResult}, err
	}
	if err := db.Exec("VACUUM").Error; err != nil {
		return StorageCleanupResult{RedisInbox: redisResult}, err
	}
	return StorageCleanupResult{RedisInbox: redisResult}, nil
}

func Vacuum(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	return db.Exec("VACUUM").Error
}
