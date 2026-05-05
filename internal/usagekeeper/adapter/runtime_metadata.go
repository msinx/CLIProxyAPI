package adapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usagekeeper/upstream/repository"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	"gorm.io/gorm"
)

type RuntimeMetadataSource struct {
	Config      *config.Config
	AuthManager *coreauth.Manager
}

func (s RuntimeMetadataSource) RefreshUsageKeeperMetadata(_ context.Context, db *gorm.DB) error {
	if db == nil {
		return nil
	}
	if err := repository.ReplaceAuthFiles(db, s.authFileInputs()); err != nil {
		return err
	}
	return repository.ReplaceProviderMetadata(db, s.providerMetadataInputs())
}

func (s RuntimeMetadataSource) authFileInputs() []repository.AuthFileInput {
	if s.AuthManager == nil {
		return nil
	}
	auths := s.AuthManager.List()
	inputs := make([]repository.AuthFileInput, 0, len(auths))
	for _, auth := range auths {
		if auth == nil {
			continue
		}
		authIndex := auth.EnsureIndex()
		if authIndex == "" {
			authIndex = auth.ID
		}
		accountType, account := auth.AccountInfo()
		inputs = append(inputs, repository.AuthFileInput{
			AuthIndex:   authIndex,
			Name:        firstRuntimeString(auth.Label, auth.FileName, auth.ID),
			Email:       emailFromAuthAccount(accountType, account),
			Type:        firstRuntimeString(accountType, auth.Provider),
			Provider:    strings.TrimSpace(auth.Provider),
			Label:       strings.TrimSpace(auth.Label),
			Status:      string(auth.Status),
			Source:      "memory",
			Disabled:    auth.Disabled,
			Unavailable: auth.Unavailable,
			RuntimeOnly: auth.FileName == "",
		})
	}
	return inputs
}

func (s RuntimeMetadataSource) providerMetadataInputs() []repository.ProviderMetadataInput {
	inputs := s.providerMetadataInputsFromAuthManager()
	if len(inputs) > 0 {
		return inputs
	}
	if s.Config == nil {
		return nil
	}
	inputs = make([]repository.ProviderMetadataInput, 0)
	inputs = appendProviderKeyMetadata(inputs, "gemini", s.Config.GeminiKey, func(item config.GeminiKey) (string, string, string) {
		return item.APIKey, item.Prefix, item.BaseURL
	})
	inputs = appendProviderKeyMetadata(inputs, "claude", s.Config.ClaudeKey, func(item config.ClaudeKey) (string, string, string) {
		return item.APIKey, item.Prefix, item.BaseURL
	})
	inputs = appendProviderKeyMetadata(inputs, "codex", s.Config.CodexKey, func(item config.CodexKey) (string, string, string) {
		return item.APIKey, item.Prefix, item.BaseURL
	})
	inputs = appendProviderKeyMetadata(inputs, "vertex", s.Config.VertexCompatAPIKey, func(item config.VertexCompatKey) (string, string, string) {
		return item.APIKey, item.Prefix, item.BaseURL
	})
	for _, compat := range s.Config.OpenAICompatibility {
		if compat.Disabled {
			continue
		}
		displayName := firstRuntimeString(compat.Name, compat.Prefix, compat.BaseURL, "openai-compatible")
		for _, entry := range compat.APIKeyEntries {
			key := strings.TrimSpace(entry.APIKey)
			if key == "" {
				continue
			}
			lookupKey := stableProviderLookupKey("openai-compatible", displayName, key)
			inputs = append(inputs, repository.ProviderMetadataInput{
				LookupKey:    lookupKey,
				ProviderType: "openai-compatible",
				DisplayName:  displayName,
				ProviderKey:  lookupKey,
				MatchKind:    "api_key_hash",
			})
		}
	}
	return inputs
}

func (s RuntimeMetadataSource) providerMetadataInputsFromAuthManager() []repository.ProviderMetadataInput {
	if s.AuthManager == nil {
		return nil
	}
	auths := s.AuthManager.List()
	inputs := make([]repository.ProviderMetadataInput, 0, len(auths))
	for _, auth := range auths {
		if auth == nil || auth.Attributes == nil {
			continue
		}
		rawKey := strings.TrimSpace(auth.Attributes["api_key"])
		if rawKey == "" {
			continue
		}
		lookupKey := firstRuntimeString(auth.Attributes["source"], stableProviderLookupKey(auth.Provider, firstRuntimeString(auth.Label, auth.Prefix), rawKey))
		displayName := firstRuntimeString(auth.Label, auth.Prefix, auth.Attributes["compat_name"], auth.Attributes["base_url"], auth.Provider)
		providerType := firstRuntimeString(auth.Attributes["provider_key"], auth.Provider)
		if lookupKey == "" || providerType == "" {
			continue
		}
		inputs = append(inputs, repository.ProviderMetadataInput{
			LookupKey:    lookupKey,
			ProviderType: providerType,
			DisplayName:  displayName,
			ProviderKey:  stableProviderKey(providerType, displayName, lookupKey),
			MatchKind:    "auth_source",
		})
	}
	return inputs
}

func appendProviderKeyMetadata[T any](inputs []repository.ProviderMetadataInput, provider string, values []T, metadata func(T) (string, string, string)) []repository.ProviderMetadataInput {
	for _, value := range values {
		key, prefix, baseURL := metadata(value)
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		displayName := firstRuntimeString(prefix, baseURL, provider)
		lookupKey := stableProviderLookupKey(provider, displayName, key)
		inputs = append(inputs, repository.ProviderMetadataInput{
			LookupKey:    lookupKey,
			ProviderType: provider,
			DisplayName:  displayName,
			ProviderKey:  lookupKey,
			MatchKind:    "api_key_hash",
		})
	}
	return inputs
}

func stableProviderLookupKey(provider, displayName, rawKey string) string {
	return stableProviderKey(provider, displayName, rawKey)
}

func stableProviderKey(provider, displayName, rawKey string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(provider) + "\n" + strings.TrimSpace(displayName) + "\n" + strings.TrimSpace(rawKey)))
	return "provider:" + strings.TrimSpace(provider) + ":" + hex.EncodeToString(sum[:8])
}

func firstRuntimeString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func emailFromAuthAccount(accountType, account string) string {
	if !strings.EqualFold(strings.TrimSpace(accountType), "oauth") {
		return ""
	}
	account = strings.TrimSpace(account)
	if at := strings.Index(account, " ("); at > 0 {
		account = strings.TrimSpace(account[:at])
	}
	if strings.Contains(account, "@") {
		return account
	}
	return ""
}

func (s RuntimeMetadataSource) UsedRegistryModelsForAuth(authID string) []string {
	models := registry.GetGlobalRegistry().GetModelsForClient(authID)
	out := make([]string, 0, len(models))
	for _, model := range models {
		if model == nil || strings.TrimSpace(model.ID) == "" {
			continue
		}
		out = append(out, strings.TrimSpace(model.ID))
	}
	return out
}
