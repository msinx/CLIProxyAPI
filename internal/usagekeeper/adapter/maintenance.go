package adapter

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/usagekeeper/upstream/backup"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/usagekeeper/upstream/entities"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/usagekeeper/upstream/repository"
	log "github.com/sirupsen/logrus"
)

func (a *App) RunMaintenance(ctx context.Context) error {
	if a == nil || a.DB == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	now := timeNow()
	if err := a.CleanupOldUsageEvents(now); err != nil {
		return err
	}
	if _, err := repository.CleanupStorage(a.DB, now); err != nil {
		return err
	}
	if err := a.AggregateUsageIdentities(ctx); err != nil {
		return err
	}
	if a.Config.BackupEnabled && a.sqlDB != nil {
		writer := backup.NewWriter(a.Config.BackupDir)
		if _, err := writer.WriteDatabase(ctx, a.sqlDB, now); err != nil {
			return err
		}
		if _, err := writer.Cleanup(a.Config.BackupRetentionDays, now); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) CleanupOldUsageEvents(now time.Time) error {
	if a == nil || a.DB == nil || a.Config.RetentionDays <= 0 {
		return nil
	}
	cutoff := now.UTC().AddDate(0, 0, -a.Config.RetentionDays)
	if err := a.DB.Where("timestamp < ?", cutoff).Delete(&entities.UsageEvent{}).Error; err != nil {
		return err
	}
	return nil
}

type MaintenanceWorker struct {
	app            *App
	interval       time.Duration
	stop           chan struct{}
	done           chan struct{}
	runMaintenance func(context.Context) error
	cancel         context.CancelFunc
	runID          uint64
	once           sync.Once
	started        atomic.Bool
	mu             sync.Mutex
}

func NewMaintenanceWorker(app *App) *MaintenanceWorker {
	interval := time.Hour
	if app != nil && app.Config.BackupInterval > 0 {
		interval = app.Config.BackupInterval
	}
	return &MaintenanceWorker{
		app:            app,
		interval:       interval,
		stop:           make(chan struct{}),
		done:           make(chan struct{}),
		runMaintenance: app.RunMaintenance,
	}
}

func (w *MaintenanceWorker) Start() {
	if w == nil || w.app == nil || w.interval <= 0 {
		return
	}
	w.started.Store(true)
	go w.run()
}

func (w *MaintenanceWorker) Stop() {
	if w == nil {
		return
	}
	if !w.started.Load() {
		return
	}
	w.once.Do(func() {
		close(w.stop)
		w.mu.Lock()
		cancel := w.cancel
		w.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		<-w.done
	})
}

func (w *MaintenanceWorker) run() {
	defer close(w.done)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithCancel(context.Background())
			w.mu.Lock()
			w.runID++
			runID := w.runID
			w.cancel = cancel
			w.mu.Unlock()
			err := w.runMaintenance(ctx)
			w.mu.Lock()
			if w.runID == runID {
				w.cancel = nil
			}
			w.mu.Unlock()
			cancel()
			if err != nil {
				log.WithError(err).Warn("usage keeper maintenance failed")
			}
		case <-w.stop:
			return
		}
	}
}

var timeNow = func() time.Time { return time.Now() }
