package adapter

import (
	"context"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usagekeeper/upstream/models"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usagekeeper/upstream/repository"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestRuntimeMetadataSourceDoesNotPersistRawProviderKeys(t *testing.T) {
	app := newTestAppWithOptions(t, Options{
		MetadataRefresher: RuntimeMetadataSource{Config: &config.Config{
			GeminiKey: []config.GeminiKey{{
				APIKey:  "gemini-secret-key",
				Prefix:  "team-a",
				BaseURL: "https://gemini.example.com",
			}},
			OpenAICompatibility: []config.OpenAICompatibility{{
				Name:    "Mirror",
				BaseURL: "https://mirror.example.com",
				APIKeyEntries: []config.OpenAICompatibilityAPIKey{{
					APIKey: "openai-secret-key",
				}},
			}},
		}},
	})

	if err := app.RefreshMetadata(context.Background()); err != nil {
		t.Fatalf("RefreshMetadata returned error: %v", err)
	}

	var items []models.UsageIdentity
	if err := app.DB.Where("auth_type = ?", models.UsageIdentityAuthTypeAIProvider).Order("type asc, provider asc").Find(&items).Error; err != nil {
		t.Fatalf("list provider usage identities: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 provider usage identities, got %d", len(items))
	}
	for _, item := range items {
		switch item.Identity {
		case "gemini-secret-key", "openai-secret-key":
			t.Fatalf("raw provider key was persisted as identity: %+v", item)
		}
		if item.Identity == "" {
			t.Fatalf("identity is empty: %+v", item)
		}
		if item.Type == "" || item.Provider == "" {
			t.Fatalf("provider identity metadata is incomplete: %+v", item)
		}
	}
}

func TestRuntimeMetadataSourceUsesAuthMetadataWithoutRawAPIKeyAccount(t *testing.T) {
	manager := coreauth.NewManager(nil, nil, nil)
	if _, err := manager.Register(context.Background(), &coreauth.Auth{
		ID:       "auth-1",
		Provider: "gemini",
		Label:    "Gemini API",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"api_key": "raw-secret-key",
			"source":  "config:gemini[token]",
		},
	}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	app := newTestAppWithOptions(t, Options{
		MetadataRefresher: RuntimeMetadataSource{AuthManager: manager},
	})
	if err := app.RefreshMetadata(context.Background()); err != nil {
		t.Fatalf("RefreshMetadata returned error: %v", err)
	}

	var files []models.UsageIdentity
	if err := app.DB.Where("auth_type = ?", models.UsageIdentityAuthTypeAuthFile).Find(&files).Error; err != nil {
		t.Fatalf("list auth usage identities: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 auth usage identity row, got %d", len(files))
	}
	if files[0].Name == "raw-secret-key" {
		t.Fatalf("raw API key account was persisted as name: %+v", files[0])
	}
	if files[0].Type != "api_key" {
		t.Fatalf("expected api_key auth type, got %+v", files[0])
	}
}

func TestPluginSourceUsesStableNonSecretProviderLookupKey(t *testing.T) {
	app := newTestApp(t)
	record := coreusage.Record{
		Provider: "gemini",
		Model:    "gemini-2.5-pro",
		APIKey:   "raw-secret-key",
		Source:   "raw-secret-key",
		Detail:   coreusage.Detail{TotalTokens: 1},
	}

	app.Plugin.HandleUsage(context.Background(), record)

	var event models.UsageEvent
	if err := app.DB.First(&event).Error; err != nil {
		t.Fatalf("load usage event: %v", err)
	}
	if event.Source == "raw-secret-key" {
		t.Fatalf("raw API key was persisted as source: %+v", event)
	}
	if event.Source == "" {
		t.Fatalf("source is empty: %+v", event)
	}

	inputs, _ := RuntimeMetadataSource{Config: &config.Config{
		GeminiKey: []config.GeminiKey{{APIKey: "raw-secret-key"}},
	}}.providerUsageIdentities()
	if len(inputs) != 1 {
		t.Fatalf("expected 1 provider usage identity input, got %d", len(inputs))
	}
	if inputs[0].Identity == "raw-secret-key" {
		t.Fatalf("raw API key was used as usage identity: %+v", inputs[0])
	}
}

func TestPluginAPIKeyAuthSourceMatchesProviderIdentity(t *testing.T) {
	app := newTestApp(t)
	manager := coreauth.NewManager(nil, nil, nil)
	if _, err := manager.Register(context.Background(), &coreauth.Auth{
		ID:       "auth-1",
		Provider: "gemini",
		Label:    "Gemini API",
		Attributes: map[string]string{
			"api_key": "raw-secret-key",
		},
	}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	auth := manager.List()[0]
	authIndex := auth.EnsureIndex()
	record := coreusage.Record{
		Provider:  "gemini",
		Model:     "gemini-2.5-pro",
		APIKey:    "raw-secret-key",
		AuthIndex: authIndex,
		AuthType:  "apikey",
		Source:    "raw-secret-key",
		Detail:    coreusage.Detail{TotalTokens: 1},
	}

	app.Plugin.HandleUsage(context.Background(), record)
	if err := (RuntimeMetadataSource{AuthManager: manager}).RefreshUsageKeeperMetadata(context.Background(), app.DB); err != nil {
		t.Fatalf("RefreshUsageKeeperMetadata returned error: %v", err)
	}
	if err := app.AggregateUsageIdentities(context.Background()); err != nil {
		t.Fatalf("AggregateUsageIdentities returned error: %v", err)
	}

	var identity models.UsageIdentity
	if err := app.DB.Where("auth_type = ? AND type = ?", models.UsageIdentityAuthTypeAIProvider, "gemini").First(&identity).Error; err != nil {
		t.Fatalf("load provider identity: %v", err)
	}
	if identity.Identity == "" || identity.Identity == "raw-secret-key" {
		t.Fatalf("provider identity is not stable and non-secret: %+v", identity)
	}
	if identity.TotalRequests != 1 {
		t.Fatalf("expected provider identity to aggregate API key auth event, got %+v", identity)
	}
}

func TestUsageIdentityReplacementDoesNotRequireRawLookupKeys(t *testing.T) {
	app := newTestApp(t)
	input := models.UsageIdentity{
		Name:         "Gemini",
		AuthType:     models.UsageIdentityAuthTypeAIProvider,
		AuthTypeName: "apikey",
		Identity:     "provider:gemini:stable",
		Type:         "gemini",
		Provider:     "Gemini",
	}
	if err := repository.ReplaceUsageIdentitiesForProviderTypes(context.Background(), app.DB, []models.UsageIdentity{input}, []string{"gemini"}, timeNow()); err != nil {
		t.Fatalf("ReplaceUsageIdentitiesForProviderTypes returned error: %v", err)
	}

	var item models.UsageIdentity
	if err := app.DB.First(&item).Error; err != nil {
		t.Fatalf("load provider usage identity: %v", err)
	}
	if item.Identity != input.Identity || item.Type != "gemini" {
		t.Fatalf("unexpected provider usage identity row: %+v", item)
	}
}
