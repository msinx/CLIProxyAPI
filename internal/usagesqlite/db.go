package usagesqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type DB struct {
	sqlDB *sql.DB
}

func OpenSQLite(ctx context.Context, path string) (*DB, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("sqlite usage path is empty")
	}
	resolvedPath, err := resolveSQLitePath(path)
	if err != nil {
		return nil, err
	}
	path = resolvedPath
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("prepare sqlite usage directory: %w", err)
	}
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite usage database: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)

	db := &DB{sqlDB: sqlDB}
	if errPragma := db.applyPragmas(ctx); errPragma != nil {
		_ = db.Close()
		return nil, errPragma
	}
	if errMigrate := db.Migrate(ctx); errMigrate != nil {
		_ = db.Close()
		return nil, errMigrate
	}
	return db, nil
}

func resolveSQLitePath(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve sqlite usage path: %w", err)
		}
		remainder := strings.TrimPrefix(path, "~")
		remainder = strings.TrimLeft(remainder, "/\\")
		if remainder == "" {
			return filepath.Clean(home), nil
		}
		normalized := strings.ReplaceAll(remainder, "\\", "/")
		return filepath.Clean(filepath.Join(home, filepath.FromSlash(normalized))), nil
	}
	return filepath.Clean(path), nil
}

func (db *DB) Close() error {
	if db == nil || db.sqlDB == nil {
		return nil
	}
	return db.sqlDB.Close()
}

func (db *DB) applyPragmas(ctx context.Context) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	}
	for _, pragma := range pragmas {
		if _, err := db.sqlDB.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("apply sqlite pragma %q: %w", pragma, err)
		}
	}
	return nil
}

func (db *DB) Migrate(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS usage_schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS usage_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_key TEXT NOT NULL UNIQUE,
			request_id TEXT NOT NULL DEFAULT '',
			timestamp INTEGER NOT NULL,
			provider TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			endpoint TEXT NOT NULL DEFAULT '',
			api_group_key TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT '',
			source_hash TEXT NOT NULL DEFAULT '',
			auth_index TEXT NOT NULL DEFAULT '',
			auth_id_hash TEXT NOT NULL DEFAULT '',
			auth_type TEXT NOT NULL DEFAULT '',
			api_key_hash TEXT NOT NULL DEFAULT '',
			failed INTEGER NOT NULL DEFAULT 0,
			status_code INTEGER NOT NULL DEFAULT 0,
			latency_ms INTEGER NOT NULL DEFAULT 0,
			input_tokens INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			reasoning_tokens INTEGER NOT NULL DEFAULT 0,
			cached_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_events_timestamp ON usage_events(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_events_model_timestamp ON usage_events(model, timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_events_provider_timestamp ON usage_events(provider, timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_events_api_group_timestamp ON usage_events(api_group_key, timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_events_source_timestamp ON usage_events(source, timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_events_source_hash_timestamp ON usage_events(source_hash, timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_events_failed_timestamp ON usage_events(failed, timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_events_auth_index_timestamp ON usage_events(auth_index, timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_events_auth_id_hash_timestamp ON usage_events(auth_id_hash, timestamp)`,
		`INSERT OR IGNORE INTO usage_schema_migrations(version, applied_at) VALUES (1, unixepoch())`,
	}
	for _, stmt := range statements {
		if _, err := db.sqlDB.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate sqlite usage database: %w", err)
		}
	}
	return nil
}
