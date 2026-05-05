package adapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usagekeeper/upstream/models"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usagekeeper/upstream/repository"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	"gorm.io/gorm"
)

type RuntimeMetadataSource struct {
	Config      *config.Config
	AuthManager *coreauth.Manager
}

func (s RuntimeMetadataSource) RefreshUsageKeeperMetadata(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now().UTC()
	if err := repository.ReplaceUsageIdentitiesForAuthType(ctx, db, s.authUsageIdentities(), models.UsageIdentityAuthTypeAuthFile, now); err != nil {
		return err
	}
	identities, providerTypes := s.providerUsageIdentities()
	return repository.ReplaceUsageIdentitiesForProviderTypes(ctx, db, identities, providerTypes, now)
}

func (s RuntimeMetadataSource) authUsageIdentities() []models.UsageIdentity {
	if s.AuthManager == nil {
		return nil
	}
	auths := s.AuthManager.List()
	identities := make([]models.UsageIdentity, 0, len(auths))
	for _, auth := range auths {
		if auth == nil {
			continue
		}
		authIndex := auth.EnsureIndex()
		if authIndex == "" {
			authIndex = auth.ID
		}
		accountType, account := auth.AccountInfo()
		identities = append(identities, models.UsageIdentity{
			Name:         firstRuntimeString(emailFromAuthAccount(accountType, account), auth.Label, auth.FileName, auth.ID, authIndex),
			AuthType:     models.UsageIdentityAuthTypeAuthFile,
			AuthTypeName: "oauth",
			Identity:     authIndex,
			Type:         firstRuntimeString(accountType, auth.Provider),
			Provider:     strings.TrimSpace(auth.Provider),
		})
	}
	return identities
}

func (s RuntimeMetadataSource) providerUsageIdentities() ([]models.UsageIdentity, []string) {
	inputs := s.providerUsageIdentitiesFromAuthManager()
	if len(inputs) > 0 {
		return inputs, providerTypesFromIdentities(inputs)
	}
	if s.Config == nil {
		return nil, nil
	}
	inputs = make([]models.UsageIdentity, 0)
	inputs = appendProviderKeyIdentity(inputs, "gemini", s.Config.GeminiKey, func(item config.GeminiKey) (string, string, string) {
		return item.APIKey, item.Prefix, item.BaseURL
	})
	inputs = appendProviderKeyIdentity(inputs, "claude", s.Config.ClaudeKey, func(item config.ClaudeKey) (string, string, string) {
		return item.APIKey, item.Prefix, item.BaseURL
	})
	inputs = appendProviderKeyIdentity(inputs, "codex", s.Config.CodexKey, func(item config.CodexKey) (string, string, string) {
		return item.APIKey, item.Prefix, item.BaseURL
	})
	inputs = appendProviderKeyIdentity(inputs, "vertex", s.Config.VertexCompatAPIKey, func(item config.VertexCompatKey) (string, string, string) {
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
			lookupKey := stableProviderLookupKey("openai-compatible", "", key)
			inputs = append(inputs, models.UsageIdentity{
				Name:         displayName,
				AuthType:     models.UsageIdentityAuthTypeAIProvider,
				AuthTypeName: "apikey",
				Identity:     lookupKey,
				Type:         "openai-compatible",
				Provider:     displayName,
			})
		}
	}
	return inputs, providerTypesFromIdentities(inputs)
}

func (s RuntimeMetadataSource) providerUsageIdentitiesFromAuthManager() []models.UsageIdentity {
	if s.AuthManager == nil {
		return nil
	}
	auths := s.AuthManager.List()
	inputs := make([]models.UsageIdentity, 0, len(auths))
	for _, auth := range auths {
		if auth == nil || auth.Attributes == nil {
			continue
		}
		rawKey := strings.TrimSpace(auth.Attributes["api_key"])
		if rawKey == "" {
			continue
		}
		lookupKey := firstRuntimeString(auth.Attributes["source"], stableProviderLookupKey(auth.Provider, "", rawKey))
		displayName := firstRuntimeString(auth.Label, auth.Prefix, auth.Attributes["compat_name"], auth.Attributes["base_url"], auth.Provider)
		providerType := firstRuntimeString(auth.Attributes["provider_key"], auth.Provider)
		if lookupKey == "" || providerType == "" {
			continue
		}
		inputs = append(inputs, models.UsageIdentity{
			Name:         displayName,
			AuthType:     models.UsageIdentityAuthTypeAIProvider,
			AuthTypeName: "apikey",
			Identity:     lookupKey,
			Type:         providerType,
			Provider:     displayName,
		})
	}
	return inputs
}

func appendProviderKeyIdentity[T any](inputs []models.UsageIdentity, provider string, values []T, metadata func(T) (string, string, string)) []models.UsageIdentity {
	for _, value := range values {
		key, prefix, baseURL := metadata(value)
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		displayName := firstRuntimeString(prefix, baseURL, provider)
		lookupKey := stableProviderLookupKey(provider, "", key)
		inputs = append(inputs, models.UsageIdentity{
			Name:         displayName,
			AuthType:     models.UsageIdentityAuthTypeAIProvider,
			AuthTypeName: "apikey",
			Identity:     lookupKey,
			Type:         provider,
			Provider:     displayName,
		})
	}
	return inputs
}

func providerTypesFromIdentities(identities []models.UsageIdentity) []string {
	types := make([]string, 0, len(identities))
	for _, identity := range identities {
		if strings.TrimSpace(identity.Type) != "" {
			types = append(types, identity.Type)
		}
	}
	return types
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
