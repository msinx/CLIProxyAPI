package usagesqlite

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

type MaintenanceOptions struct {
	Now                 time.Time
	RetentionDays       int
	BackupEnabled       bool
	BackupDir           string
	BackupRetentionDays int
}

type MaintenanceResult struct {
	DeletedEvents int64
	BackupPath    string
}

func StartRetentionCleaner(ctx context.Context, store *Store, retentionDays int, interval time.Duration) {
	StartMaintenanceWorker(ctx, store, MaintenanceOptions{RetentionDays: retentionDays}, interval)
}

func StartMaintenanceWorker(ctx context.Context, store *Store, opts MaintenanceOptions, interval time.Duration) {
	if ctx == nil {
		ctx = context.Background()
	}
	if store == nil || !opts.enabled() {
		return
	}
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	clean := func() {
		result, err := RunMaintenance(context.Background(), store, opts)
		if err != nil {
			log.WithError(err).Warn("failed to run sqlite usage maintenance")
			return
		}
		if result.DeletedEvents > 0 {
			log.WithField("deleted", result.DeletedEvents).Info("cleaned old sqlite usage events")
		}
		if result.BackupPath != "" {
			log.WithField("path", result.BackupPath).Info("created sqlite usage backup")
		}
	}
	go func() {
		clean()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				clean()
			}
		}
	}()
}

func RunMaintenance(ctx context.Context, store *Store, opts MaintenanceOptions) (MaintenanceResult, error) {
	var result MaintenanceResult
	if ctx == nil {
		ctx = context.Background()
	}
	if store == nil {
		return result, fmt.Errorf("sqlite usage store is not initialized")
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	if opts.RetentionDays > 0 {
		cutoff := now.AddDate(0, 0, -opts.RetentionDays)
		deleted, err := store.DeleteBefore(ctx, cutoff)
		if err != nil {
			return result, err
		}
		result.DeletedEvents = deleted
		if deleted > 0 {
			if errVacuum := store.Vacuum(ctx); errVacuum != nil {
				return result, errVacuum
			}
		}
	}
	if errCheckpoint := store.Checkpoint(ctx); errCheckpoint != nil {
		return result, errCheckpoint
	}
	if opts.BackupRetentionDays >= 0 && strings.TrimSpace(opts.BackupDir) != "" {
		if err := pruneBackups(opts.BackupDir, now.AddDate(0, 0, -opts.BackupRetentionDays)); err != nil {
			return result, err
		}
	}
	if opts.BackupEnabled {
		backupPath, err := store.BackupToDir(ctx, opts.BackupDir, now)
		if err != nil {
			return result, err
		}
		result.BackupPath = backupPath
	}
	return result, nil
}

func (opts MaintenanceOptions) enabled() bool {
	return opts.RetentionDays > 0 || opts.BackupEnabled || (opts.BackupRetentionDays >= 0 && strings.TrimSpace(opts.BackupDir) != "")
}

func (s *Store) Checkpoint(ctx context.Context) error {
	if s == nil || s.db == nil || s.db.sqlDB == nil {
		return fmt.Errorf("sqlite usage store is not initialized")
	}
	if _, err := s.db.sqlDB.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return fmt.Errorf("checkpoint sqlite usage database: %w", err)
	}
	return nil
}

func (s *Store) Vacuum(ctx context.Context) error {
	if s == nil || s.db == nil || s.db.sqlDB == nil {
		return fmt.Errorf("sqlite usage store is not initialized")
	}
	if _, err := s.db.sqlDB.ExecContext(ctx, `VACUUM`); err != nil {
		return fmt.Errorf("vacuum sqlite usage database: %w", err)
	}
	return nil
}

func (s *Store) BackupToDir(ctx context.Context, dir string, now time.Time) (string, error) {
	if s == nil || s.db == nil || s.db.sqlDB == nil {
		return "", fmt.Errorf("sqlite usage store is not initialized")
	}
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "", fmt.Errorf("sqlite usage backup path is empty")
	}
	resolvedDir, err := resolveSQLitePath(dir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(resolvedDir, 0o755); err != nil {
		return "", fmt.Errorf("prepare sqlite usage backup directory: %w", err)
	}
	if now.IsZero() {
		now = time.Now()
	}
	target := filepath.Join(resolvedDir, "usage-backup-"+now.UTC().Format("20060102150405")+".db")
	target, err = nextBackupPath(target)
	if err != nil {
		return "", err
	}
	if _, err := s.db.sqlDB.ExecContext(ctx, `VACUUM INTO ?`, target); err != nil {
		return "", fmt.Errorf("backup sqlite usage database: %w", err)
	}
	return target, nil
}

func nextBackupPath(path string) (string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path, nil
	} else if err != nil {
		return "", fmt.Errorf("inspect sqlite usage backup path: %w", err)
	}
	ext := filepath.Ext(path)
	prefix := strings.TrimSuffix(path, ext)
	for i := 1; i < 1000; i++ {
		candidate := fmt.Sprintf("%s-%03d%s", prefix, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		} else if err != nil {
			return "", fmt.Errorf("inspect sqlite usage backup path: %w", err)
		}
	}
	return "", fmt.Errorf("sqlite usage backup path collision limit reached")
}

func pruneBackups(dir string, cutoff time.Time) error {
	resolvedDir, err := resolveSQLitePath(dir)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(resolvedDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list sqlite usage backups: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "usage-backup-") || filepath.Ext(entry.Name()) != ".db" {
			continue
		}
		info, errInfo := entry.Info()
		if errInfo != nil {
			return fmt.Errorf("inspect sqlite usage backup: %w", errInfo)
		}
		if info.ModTime().Before(cutoff) {
			if errRemove := os.Remove(filepath.Join(resolvedDir, entry.Name())); errRemove != nil {
				return fmt.Errorf("remove old sqlite usage backup: %w", errRemove)
			}
		}
	}
	return nil
}
