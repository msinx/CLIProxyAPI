package adapter

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	upstreamauth "github.com/router-for-me/CLIProxyAPI/v7/internal/usagekeeper/upstream/auth"
	upstreamconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/usagekeeper/upstream/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/usagekeeper/upstream/poller"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/usagekeeper/upstream/repository"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/usagekeeper/upstream/service"
	coreusage "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type App struct {
	Config                Config
	DB                    *gorm.DB
	UsageProvider         service.UsageProvider
	UsageIdentityProvider service.UsageIdentityProvider
	PricingProvider       service.PricingProvider
	Plugin                *Plugin
	Sessions              *upstreamauth.SessionManager

	sqlDB     *sql.DB
	status    *StatusProvider
	store     *usageStore
	metadata  MetadataRefresher
	maint     *MaintenanceWorker
	closeOnce sync.Once
	closeErr  error
}

type Options struct {
	ConfigDir         string
	Enabled           *bool
	MetadataRefresher MetadataRefresher
}

var (
	singletonPlugin    = NewPlugin()
	registerPluginOnce sync.Once
)

func NewApp(cfg Config) (*App, error) {
	enabled := true
	return NewAppWithOptions(cfg, Options{Enabled: &enabled})
}

func NewAppWithOptions(cfg Config, opts Options) (*App, error) {
	if opts.Enabled != nil {
		cfg.Enabled = *opts.Enabled
	}
	normalized, err := NormalizeConfig(cfg, opts.ConfigDir)
	if err != nil {
		return nil, err
	}
	registerPluginOnce.Do(func() {
		coreusage.RegisterPlugin(singletonPlugin)
	})

	app := &App{
		Config:   normalized,
		Plugin:   singletonPlugin,
		Sessions: upstreamauth.NewSessionManager(normalized.SessionTTL),
		status:   NewStatusProvider(normalized.Timezone),
		metadata: opts.MetadataRefresher,
	}
	if !normalized.Enabled {
		app.Plugin.Disable()
		return app, nil
	}

	if err := os.MkdirAll(filepathDir(normalized.DatabasePath), 0o755); err != nil {
		return nil, fmt.Errorf("create usage keeper database directory: %w", err)
	}
	db, err := repository.OpenDatabase(upstreamconfig.Config{SQLitePath: normalized.DatabasePath})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		if closeErr := closeGormDB(db); closeErr != nil {
			log.WithError(closeErr).Warn("usage keeper database close after initialization failure failed")
		}
		return nil, fmt.Errorf("get usage keeper sql database: %w", err)
	}

	app.DB = db
	app.sqlDB = sqlDB
	app.UsageProvider = service.NewUsageService(db)
	app.UsageIdentityProvider = service.NewUsageIdentityService(db)
	app.PricingProvider = service.NewPricingService(db)
	app.store = &usageStore{db: db}
	app.Plugin.SetStore(app.store)
	app.Plugin.Enable()
	app.maint = NewMaintenanceWorker(app)
	app.maint.Start()

	return app, nil
}

func (a *App) Close() error {
	if a == nil {
		return nil
	}
	a.closeOnce.Do(func() {
		if a.maint != nil {
			a.maint.Stop()
		}
		if a.Plugin != nil {
			a.Plugin.DisableStore(a.store, defaultDrainTimeout)
		}
		if a.sqlDB != nil {
			if err := a.sqlDB.Close(); err != nil {
				a.closeErr = fmt.Errorf("close usage keeper database: %w", err)
				return
			}
		}
		log.Debug("usage keeper adapter closed")
	})
	return a.closeErr
}

func (a *App) EnableIngestion() {
	if a == nil || a.Plugin == nil || a.store == nil {
		return
	}
	a.Plugin.SetStore(a.store)
	a.Plugin.Enable()
}

func (a *App) DisableIngestion() {
	if a == nil || a.Plugin == nil {
		return
	}
	a.Plugin.DisableStore(a.store, defaultDrainTimeout)
}

func (a *App) InvalidateSessions() {
	if a == nil || a.Sessions == nil {
		return
	}
	a.Sessions.InvalidateAll()
}

func (a *App) SetMetadataRefresher(refresher MetadataRefresher) {
	if a == nil {
		return
	}
	a.metadata = refresher
}

func closeGormDB(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (a *App) SyncNow(ctx context.Context) error {
	if a == nil || a.status == nil {
		return nil
	}
	if err := a.RefreshMetadata(ctx); err != nil {
		a.status.MarkSyncWarning(err)
		return err
	}
	if err := a.AggregateUsageIdentities(ctx); err != nil {
		a.status.MarkSyncWarning(err)
		return err
	}
	a.status.MarkSyncCompleted()
	return nil
}

func (a *App) Status() poller.Status {
	if a == nil || a.status == nil {
		return poller.Status{LastStatus: "completed"}
	}
	return a.status.Status()
}

func filepathDir(path string) string {
	cleaned := filepath.Clean(path)
	if cleaned == "." {
		return "."
	}
	return filepath.Dir(cleaned)
}

func (a *App) RefreshMetadata(ctx context.Context) error {
	if a == nil || a.DB == nil || a.metadata == nil {
		return nil
	}
	return a.metadata.RefreshUsageKeeperMetadata(ctx, a.DB)
}

func (a *App) AggregateUsageIdentities(ctx context.Context) error {
	if a == nil || a.DB == nil {
		return nil
	}
	return repository.AggregateUsageIdentityStats(ctx, a.DB, time.Now().UTC())
}
