package usagesqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateCreatesUsageTablesAndIndexes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "usage.db"))
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	defer closeTestDB(t, db)

	assertTableExists(t, db.sqlDB, "usage_events")
	assertTableExists(t, db.sqlDB, "usage_schema_migrations")
	assertIndexExists(t, db.sqlDB, "idx_usage_events_timestamp")
	assertIndexExists(t, db.sqlDB, "idx_usage_events_model_timestamp")
	assertIndexExists(t, db.sqlDB, "idx_usage_events_provider_timestamp")
}

func TestOpenSQLiteExpandsHomeDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	ctx := context.Background()
	db, err := OpenSQLite(ctx, "~/usage-test/usage.db")
	if err != nil {
		t.Fatalf("OpenSQLite() error = %v", err)
	}
	defer closeTestDB(t, db)

	wantPath := filepath.Join(os.Getenv("HOME"), "usage-test", "usage.db")
	if _, errStat := os.Stat(wantPath); errStat != nil {
		t.Fatalf("expected sqlite database at %s: %v", wantPath, errStat)
	}
}

func closeTestDB(t *testing.T, db *DB) {
	t.Helper()
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func assertTableExists(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	var got string
	err := db.QueryRow("SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", name).Scan(&got)
	if err != nil {
		t.Fatalf("table %s not found: %v", name, err)
	}
}

func assertIndexExists(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	var got string
	err := db.QueryRow("SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?", name).Scan(&got)
	if err != nil {
		t.Fatalf("index %s not found: %v", name, err)
	}
}
