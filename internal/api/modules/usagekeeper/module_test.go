package usagekeeper

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/api/modules"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/util"
)

func TestUsageKeeperModuleMountsPublicShellAndProtectsAPIs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	module := New(ModuleOptions{
		ConfigDir: t.TempDir(),
		Verifier: func(_ string, _ bool, provided string) (bool, int, string) {
			if provided == "management-secret" {
				return true, 0, ""
			}
			return false, http.StatusUnauthorized, "invalid management key"
		},
	})
	t.Cleanup(func() {
		if err := module.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	})

	cfg := &config.Config{UsageStatisticsEnabled: false}
	if err := module.Register(modules.Context{Engine: engine, Config: cfg}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if module.app == nil || module.app.DB == nil {
		t.Fatal("usage keeper app/database should start even when usage statistics ingestion is disabled")
	}

	req := httptest.NewRequest(http.MethodGet, "/usage", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("GET /usage status = %d, want 301; body=%s", w.Code, w.Body.String())
	}
	if location := w.Header().Get("Location"); location != "/usage/" {
		t.Fatalf("GET /usage location = %q, want /usage/", location)
	}
	req = httptest.NewRequest(http.MethodGet, "/usage/", nil)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /usage/ status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "CPA USAGE KEEPER") || !strings.Contains(w.Body.String(), `window.__APP_BASE_PATH__ = "/usage"`) {
		t.Fatalf("GET /usage/ did not return dashboard shell: %s", w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/usage/api/v1/status", nil)
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status code = %d, want 401", w.Code)
	}

	loginReq := httptest.NewRequest(http.MethodPost, "/usage/api/v1/auth/login", bytes.NewBufferString(`{"password":"management-secret"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginReq.RemoteAddr = "127.0.0.1:12345"
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, loginReq)
	if w.Code != http.StatusNoContent {
		t.Fatalf("login status = %d, want 204; body=%s", w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("login did not set a session cookie")
	}
	if cookies[0].Path != "/usage" || !cookies[0].HttpOnly {
		t.Fatalf("unexpected cookie attributes: %+v", cookies[0])
	}

	req = httptest.NewRequest(http.MethodGet, "/usage/api/v1/status", nil)
	req.AddCookie(cookies[0])
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("authenticated status code = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

func TestUsageKeeperModuleRegisterIsIdempotent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	module := New(ModuleOptions{ConfigDir: t.TempDir()})
	t.Cleanup(func() {
		if err := module.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	})

	ctx := modules.Context{Engine: engine, Config: &config.Config{}}
	if err := module.Register(ctx); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if err := module.Register(ctx); err != nil {
		t.Fatalf("second Register returned error: %v", err)
	}
}

func TestUsageKeeperModuleResolvesTildeAuthDir(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	module := New(ModuleOptions{
		ConfigDir: t.TempDir(),
	})
	t.Cleanup(func() {
		if err := module.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	})
	cfg := &config.Config{
		AuthDir: "~/.cli-proxy-api",
	}
	if err := module.Register(modules.Context{Engine: engine, Config: cfg}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if module.app == nil {
		t.Fatal("module app not initialized")
	}
	resolvedAuthDir, err := util.ResolveAuthDir(cfg.AuthDir)
	if err != nil {
		t.Fatalf("ResolveAuthDir returned error: %v", err)
	}
	want := filepath.Join(filepath.Dir(resolvedAuthDir), "data", "usage-keeper.db")
	if got := module.app.Config.DatabasePath; got != want {
		t.Fatalf("DatabasePath = %q, want %q", got, want)
	}
}

func TestUsageKeeperModuleInvalidatesSessionsOnRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	module := New(ModuleOptions{
		ConfigDir: t.TempDir(),
		Verifier: func(_ string, _ bool, provided string) (bool, int, string) {
			return provided == "management-secret", http.StatusUnauthorized, "invalid management key"
		},
	})
	cfg := &config.Config{UsageStatisticsEnabled: true}
	if err := module.Register(modules.Context{Engine: engine, Config: cfg}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	loginReq := httptest.NewRequest(http.MethodPost, "/usage/api/v1/auth/login", bytes.NewBufferString(`{"password":"management-secret"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginReq.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, loginReq)
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie")
	}
	req := httptest.NewRequest(http.MethodGet, "/usage/api/v1/status", nil)
	req.AddCookie(cookies[0])
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected authenticated request to succeed, got %d", w.Code)
	}
	module.InvalidateSessions()
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected invalidated session to be rejected, got %d", w.Code)
	}
}
