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

	var items []models.ProviderMetadata
	if err := app.DB.Order("provider_type asc, display_name asc").Find(&items).Error; err != nil {
		t.Fatalf("list provider metadata: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 provider metadata rows, got %d", len(items))
	}
	for _, item := range items {
		switch item.LookupKey {
		case "gemini-secret-key", "openai-secret-key":
			t.Fatalf("raw provider key was persisted as lookup key: %+v", item)
		}
		if item.LookupKey == "" {
			t.Fatalf("lookup key is empty: %+v", item)
		}
		if item.ProviderKey == "" {
			t.Fatalf("provider key is empty: %+v", item)
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

	var files []models.AuthFile
	if err := app.DB.Find(&files).Error; err != nil {
		t.Fatalf("list auth files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 auth file metadata row, got %d", len(files))
	}
	if files[0].Email == "raw-secret-key" {
		t.Fatalf("raw API key account was persisted as email: %+v", files[0])
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

	inputs := RuntimeMetadataSource{Config: &config.Config{
		GeminiKey: []config.GeminiKey{{APIKey: "raw-secret-key"}},
	}}.providerMetadataInputs()
	if len(inputs) != 1 {
		t.Fatalf("expected 1 provider metadata input, got %d", len(inputs))
	}
	if inputs[0].LookupKey == "raw-secret-key" {
		t.Fatalf("raw API key was used as metadata lookup key: %+v", inputs[0])
	}
}

func TestProviderMetadataReplacementDoesNotRequireRawLookupKeys(t *testing.T) {
	app := newTestApp(t)
	input := repository.ProviderMetadataInput{
		LookupKey:    "provider:gemini:stable",
		ProviderType: "gemini",
		DisplayName:  "Gemini",
		ProviderKey:  "provider:gemini:stable",
		MatchKind:    "api_key_hash",
	}
	if err := repository.ReplaceProviderMetadata(app.DB, []repository.ProviderMetadataInput{input}); err != nil {
		t.Fatalf("ReplaceProviderMetadata returned error: %v", err)
	}

	var item models.ProviderMetadata
	if err := app.DB.First(&item).Error; err != nil {
		t.Fatalf("load provider metadata: %v", err)
	}
	if item.LookupKey != input.LookupKey || item.MatchKind != "api_key_hash" {
		t.Fatalf("unexpected provider metadata row: %+v", item)
	}
}
