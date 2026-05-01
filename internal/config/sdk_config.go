// Package config provides configuration management for the CLI Proxy API server.
// It handles loading and parsing YAML configuration files, and provides structured
// access to application settings including server port, authentication directory,
// debug settings, proxy configuration, and API keys.
package config

import "strings"

// SDKConfig represents the application's configuration, loaded from a YAML file.
type SDKConfig struct {
	// ProxyURL is the URL of an optional proxy server to use for outbound requests.
	ProxyURL string `yaml:"proxy-url" json:"proxy-url"`

	// DisableImageGeneration controls whether the built-in image_generation tool is injected/allowed.
	//
	// Supported values:
	//   - false (default): image_generation is enabled everywhere (normal behavior).
	//   - true: image_generation is disabled everywhere. The server stops injecting it, removes it from request payloads,
	//     and returns 404 for /v1/images/generations and /v1/images/edits.
	//   - "chat": disable image_generation injection for all non-images endpoints (e.g. /v1/responses, /v1/chat/completions),
	//     while keeping /v1/images/generations and /v1/images/edits enabled and preserving image_generation there.
	DisableImageGeneration DisableImageGenerationMode `yaml:"disable-image-generation" json:"disable-image-generation"`

	// EnableGeminiCLIEndpoint controls whether Gemini CLI internal endpoints (/v1internal:*) are enabled.
	// Default is false for safety; when false, /v1internal:* requests are rejected.
	EnableGeminiCLIEndpoint bool `yaml:"enable-gemini-cli-endpoint" json:"enable-gemini-cli-endpoint"`

	// ForceModelPrefix requires explicit model prefixes (e.g., "teamA/gemini-3-pro-preview")
	// to target prefixed credentials. When false, unprefixed model requests may use prefixed
	// credentials as well.
	ForceModelPrefix bool `yaml:"force-model-prefix" json:"force-model-prefix"`

	// RequestLog enables or disables detailed request logging functionality.
	RequestLog bool `yaml:"request-log" json:"request-log"`

	// APIKeys is a list of keys for authenticating clients to this proxy server.
	APIKeys []string `yaml:"api-keys" json:"api-keys"`

	// apiKeyEntries stores optional metadata loaded from object-form api-keys
	// entries. It is intentionally excluded from YAML/JSON so api-keys remains
	// the single public config representation and management/config responses do
	// not accidentally duplicate raw keys.
	apiKeyEntries []APIKeyEntry

	// PassthroughHeaders controls whether upstream response headers are forwarded to downstream clients.
	// Default is false (disabled).
	PassthroughHeaders bool `yaml:"passthrough-headers" json:"passthrough-headers"`

	// Streaming configures server-side streaming behavior (keep-alives and safe bootstrap retries).
	Streaming StreamingConfig `yaml:"streaming" json:"streaming"`

	// NonStreamKeepAliveInterval controls how often blank lines are emitted for non-streaming responses.
	// <= 0 disables keep-alives. Value is in seconds.
	NonStreamKeepAliveInterval int `yaml:"nonstream-keepalive-interval,omitempty" json:"nonstream-keepalive-interval,omitempty"`
}

// APIKeyEntry describes a top-level client API key plus optional display
// metadata. APIKey is the only value used for authentication; Alias, Name, and
// Comment are presentation metadata for usage attribution.
type APIKeyEntry struct {
	APIKey  string `yaml:"api-key" json:"api-key"`
	Alias   string `yaml:"alias,omitempty" json:"alias,omitempty"`
	Name    string `yaml:"name,omitempty" json:"name,omitempty"`
	Comment string `yaml:"comment,omitempty" json:"comment,omitempty"`
}

// RawAPIKeys returns a defensive copy of the configured client API keys.
func (cfg *SDKConfig) RawAPIKeys() []string {
	if cfg == nil || len(cfg.APIKeys) == 0 {
		return nil
	}
	return append([]string(nil), cfg.APIKeys...)
}

// APIKeyEntries returns a defensive copy of rich API key entries in raw-key
// order. Programmatically configured keys without metadata are returned as
// string-equivalent entries.
func (cfg *SDKConfig) APIKeyEntries() []APIKeyEntry {
	if cfg == nil {
		return nil
	}
	return cfg.apiKeyEntriesForMarshal()
}

// APIKeyMetadataByKey returns metadata keyed by normalized raw API key. For
// duplicate raw keys, the first non-empty metadata wins.
func (cfg *SDKConfig) APIKeyMetadataByKey() map[string]APIKeyEntry {
	if cfg == nil {
		return nil
	}
	out := make(map[string]APIKeyEntry, len(cfg.APIKeys))
	for _, entry := range cfg.apiKeyEntries {
		key := normalizeAPIKeyValue(entry.APIKey)
		if key == "" {
			continue
		}
		if _, exists := out[key]; exists {
			continue
		}
		if !entry.HasMetadata() {
			continue
		}
		entry.APIKey = key
		out[key] = entry
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// APIKeyMetadata returns display metadata for a raw API key.
func (cfg *SDKConfig) APIKeyMetadata(key string) (APIKeyEntry, bool) {
	metadata := cfg.APIKeyMetadataByKey()
	if len(metadata) == 0 {
		return APIKeyEntry{}, false
	}
	entry, ok := metadata[normalizeAPIKeyValue(key)]
	return entry, ok
}

// HasAPIKey reports whether key is present in the top-level client API key
// list after normalization.
func (cfg *SDKConfig) HasAPIKey(key string) bool {
	needle := normalizeAPIKeyValue(key)
	if cfg == nil || needle == "" {
		return false
	}
	for _, candidate := range cfg.APIKeys {
		if normalizeAPIKeyValue(candidate) == needle {
			return true
		}
	}
	return false
}

// DisplayLabelForAPIKey returns the human-facing usage label for key.
// Alias wins over Name, then the masked key is used as a fallback. When an
// alias/name exists, the masked key is appended to keep duplicate aliases
// distinguishable without exposing the full secret.
func (cfg *SDKConfig) DisplayLabelForAPIKey(key string) string {
	masked := MaskAPIKey(key)
	if entry, ok := cfg.APIKeyMetadata(key); ok {
		label := SafeAPIKeyMetadataText(key, entry.Alias)
		if label == "" {
			label = SafeAPIKeyMetadataText(key, entry.Name)
		}
		if label != "" {
			if masked != "" {
				return label + " (" + masked + ")"
			}
			return label
		}
	}
	return masked
}

func SafeAPIKeyMetadataText(key string, value string) string {
	trimmed := strings.TrimSpace(value)
	key = normalizeAPIKeyValue(key)
	if trimmed == "" || key == "" {
		return trimmed
	}
	return strings.TrimSpace(strings.ReplaceAll(trimmed, key, MaskAPIKey(key)))
}

// MaskAPIKey masks a client API key while preserving a short prefix/suffix for
// troubleshooting. It mirrors the TUI masking behavior.
func MaskAPIKey(key string) string {
	if len(key) <= 8 {
		return strings.Repeat("*", len(key))
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}

// HasMetadata reports whether an API key entry carries display metadata.
func (entry APIKeyEntry) HasMetadata() bool {
	return strings.TrimSpace(entry.Alias) != "" ||
		strings.TrimSpace(entry.Name) != "" ||
		strings.TrimSpace(entry.Comment) != ""
}

func (cfg *SDKConfig) setAPIKeyEntries(entries []APIKeyEntry) {
	if cfg == nil {
		return
	}
	if len(entries) == 0 {
		cfg.apiKeyEntries = nil
		return
	}
	cfg.apiKeyEntries = append([]APIKeyEntry(nil), entries...)
}

func (cfg *SDKConfig) apiKeyEntriesForMarshal() []APIKeyEntry {
	if cfg == nil || len(cfg.APIKeys) == 0 {
		return nil
	}
	metadata := cfg.APIKeyMetadataByKey()
	entries := make([]APIKeyEntry, 0, len(cfg.APIKeys))
	for _, rawKey := range cfg.APIKeys {
		entry := APIKeyEntry{APIKey: rawKey}
		if meta, ok := metadata[normalizeAPIKeyValue(rawKey)]; ok {
			entry.Alias = strings.TrimSpace(meta.Alias)
			entry.Name = strings.TrimSpace(meta.Name)
			entry.Comment = strings.TrimSpace(meta.Comment)
		}
		entries = append(entries, entry)
	}
	return entries
}

func normalizeAPIKeyValue(key string) string {
	return strings.TrimSpace(key)
}

// StreamingConfig holds server streaming behavior configuration.
type StreamingConfig struct {
	// KeepAliveSeconds controls how often the server emits SSE heartbeats (": keep-alive\n\n").
	// <= 0 disables keep-alives. Default is 0.
	KeepAliveSeconds int `yaml:"keepalive-seconds,omitempty" json:"keepalive-seconds,omitempty"`

	// BootstrapRetries controls how many times the server may retry a streaming request before any bytes are sent,
	// to allow auth rotation / transient recovery.
	// <= 0 disables bootstrap retries. Default is 0.
	BootstrapRetries int `yaml:"bootstrap-retries,omitempty" json:"bootstrap-retries,omitempty"`
}
