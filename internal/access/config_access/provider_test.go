package configaccess

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func TestProviderAuthenticatesObjectFormAPIKeyValue(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte(`api-keys:
  - api-key: real-key
    alias: Team Alias
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := internalconfig.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	provider := newProvider("test", cfg.SDKConfig.RawAPIKeys())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer real-key")

	result, authErr := provider.Authenticate(req.Context(), req)
	if authErr != nil {
		t.Fatalf("Authenticate() error = %v", authErr)
	}
	if result == nil || result.Principal != "real-key" {
		t.Fatalf("result = %#v, want principal real-key", result)
	}
}

func TestProviderDoesNotAuthenticateAliasMetadata(t *testing.T) {
	provider := newProvider("test", []string{"real-key"})
	tests := []struct {
		name  string
		apply func(*http.Request)
	}{
		{"authorization", func(r *http.Request) { r.Header.Set("Authorization", "Bearer Team Alias") }},
		{"x-goog-api-key", func(r *http.Request) { r.Header.Set("X-Goog-Api-Key", "Team Alias") }},
		{"x-api-key", func(r *http.Request) { r.Header.Set("X-Api-Key", "Team Alias") }},
		{"query-key", func(r *http.Request) { q := r.URL.Query(); q.Set("key", "Team Alias"); r.URL.RawQuery = q.Encode() }},
		{"query-auth-token", func(r *http.Request) {
			q := url.Values{}
			q.Set("auth_token", "Team Alias")
			r.URL.RawQuery = q.Encode()
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			tt.apply(req)
			result, authErr := provider.Authenticate(req.Context(), req)
			if authErr == nil || result != nil {
				t.Fatalf("Authenticate() = (%#v, %v), want rejection", result, authErr)
			}
		})
	}
}
