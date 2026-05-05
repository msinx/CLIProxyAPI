package usagekeeper

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/api/modules"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usagekeeper/adapter"
	usageassets "github.com/router-for-me/CLIProxyAPI/v6/internal/usagekeeper/assets"
	upstreamapi "github.com/router-for-me/CLIProxyAPI/v6/internal/usagekeeper/upstream/api"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/util"
)

type ModuleOptions struct {
	Verifier          adapter.ManagementKeyVerifier
	ConfigDir         string
	MetadataRefresher adapter.MetadataRefresher
}

type Module struct {
	verifier          adapter.ManagementKeyVerifier
	configDir         string
	metadataRefresher adapter.MetadataRefresher
	app               *adapter.App

	registerOnce sync.Once
	registerErr  error
}

func New(opts ModuleOptions) *Module {
	return &Module{verifier: opts.Verifier, configDir: opts.ConfigDir, metadataRefresher: opts.MetadataRefresher}
}

func (m *Module) Name() string {
	return "usage-keeper"
}

func (m *Module) Register(ctx modules.Context) error {
	if m == nil {
		return fmt.Errorf("usage keeper module is nil")
	}
	m.registerOnce.Do(func() {
		m.registerErr = m.register(ctx)
	})
	return m.registerErr
}

func (m *Module) register(ctx modules.Context) error {
	if ctx.Engine == nil {
		return fmt.Errorf("usage keeper module requires a gin engine")
	}
	cfg := ctx.Config
	appCfg := adapter.DefaultConfig()
	configDir := "."
	if optionDir := strings.TrimSpace(m.configDir); optionDir != "" {
		configDir = optionDir
	}
	if cfg != nil {
		if authDir := strings.TrimSpace(cfg.AuthDir); authDir != "" {
			if resolvedAuthDir, errResolve := util.ResolveAuthDir(authDir); errResolve == nil && resolvedAuthDir != "" {
				configDir = filepath.Dir(resolvedAuthDir)
			} else {
				configDir = filepath.Dir(authDir)
			}
		}
	}
	app, err := adapter.NewAppWithOptions(appCfg, adapter.Options{ConfigDir: configDir, MetadataRefresher: m.metadataRefresher})
	if err != nil {
		return err
	}
	if cfg != nil && !cfg.UsageStatisticsEnabled {
		app.DisableIngestion()
	}
	m.app = app

	root := ctx.Engine.Group(app.Config.BasePath)
	root.GET("", m.serveIndex)
	root.GET("/", m.serveIndex)
	root.GET("/assets/*filepath", m.serveAsset)
	authHandler := upstreamapi.NewAuthHandler(upstreamapi.AuthConfig{
		Enabled:    true,
		SessionTTL: app.Config.SessionTTL,
		BasePath:   app.Config.BasePath,
		Verifier:   m.verifier,
	}, app.Sessions)
	authConfig := upstreamapi.AuthConfig{
		Enabled:    true,
		SessionTTL: app.Config.SessionTTL,
		BasePath:   app.Config.BasePath,
		Verifier:   m.verifier,
	}
	upstreamapi.RegisterEmbeddedRoutes(
		root.Group("/api/v1"),
		app,
		app.UsageProvider,
		app.PricingProvider,
		authConfig,
		authHandler,
		app.UsageIdentityProvider,
	)
	root.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

	return nil
}

func (m *Module) InvalidateSessions() {
	if m == nil || m.app == nil {
		return
	}
	m.app.InvalidateSessions()
}

func (m *Module) OnConfigUpdated(cfg *config.Config) error {
	if m == nil || m.app == nil {
		return nil
	}
	if cfg == nil || cfg.UsageStatisticsEnabled {
		m.app.EnableIngestion()
		return nil
	}
	m.app.DisableIngestion()
	return nil
}

func (m *Module) SetMetadataRefresher(refresher adapter.MetadataRefresher) {
	if m == nil {
		return
	}
	m.metadataRefresher = refresher
	if m.app != nil {
		m.app.SetMetadataRefresher(refresher)
	}
}

func (m *Module) Close() error {
	if m == nil || m.app == nil {
		return nil
	}
	return m.app.Close()
}

func (m *Module) serveIndex(c *gin.Context) {
	data, err := usageassets.FS.ReadFile("dist/index.html")
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	data = bytes.ReplaceAll(data, []byte(`"__APP_BASE_PATH__"`), []byte(strconv.Quote(m.app.Config.BasePath)))
	c.Data(http.StatusOK, "text/html; charset=utf-8", data)
}

func (m *Module) serveAsset(c *gin.Context) {
	name := strings.TrimPrefix(c.Param("filepath"), "/")
	if name == "" {
		c.Status(http.StatusNotFound)
		return
	}
	file, err := usageassets.FS.Open("dist/assets/" + name)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer func() {
		if errClose := file.Close(); errClose != nil {
			_ = errClose
		}
	}()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		c.Status(http.StatusNotFound)
		return
	}
	data, err := io.ReadAll(file)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Data(http.StatusOK, contentType(name), data)
}

func contentType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	default:
		return "application/octet-stream"
	}
}
