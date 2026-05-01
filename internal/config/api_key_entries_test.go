package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoadConfigOptional_APIKeysMixedEntries(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := `api-keys:
  - legacy-key
  - api-key: object-key
    alias: Team Alias
    name: Team Name
    comment: Used by tests
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfigOptional(configPath, false)
	if err != nil {
		t.Fatalf("LoadConfigOptional() error = %v", err)
	}
	if got, want := cfg.APIKeys, []string{"legacy-key", "object-key"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("APIKeys = %#v, want %#v", got, want)
	}
	entry, ok := cfg.SDKConfig.APIKeyMetadata(" object-key ")
	if !ok {
		t.Fatalf("expected metadata for object-key")
	}
	if entry.Alias != "Team Alias" || entry.Name != "Team Name" || entry.Comment != "Used by tests" {
		t.Fatalf("metadata = %#v", entry)
	}
	if got := cfg.SDKConfig.DisplayLabelForAPIKey("object-key"); got != "Team Alias (obje**-key)" {
		t.Fatalf("DisplayLabelForAPIKey() = %q", got)
	}
}

func TestConfigMarshalYAML_APIKeysMixedEntries(t *testing.T) {
	var cfg Config
	if err := yaml.Unmarshal([]byte(`api-keys:
  - legacy-key
  - api-key: object-key
    alias: Team Alias
    comment: Used by tests
`), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}
	out := string(data)
	for _, want := range []string{"api-key: object-key", "alias: Team Alias", "comment: Used by tests", "- legacy-key"} {
		if !strings.Contains(out, want) {
			t.Fatalf("marshaled YAML missing %q:\n%s", want, out)
		}
	}
}

func TestSaveConfigPreserveComments_APIKeyMetadataSurvivesUnrelatedSave(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	content := `# config
api-keys:
  - api-key: object-key
    alias: Team Alias
    comment: Used by tests
debug: false
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.Debug = true
	if err := SaveConfigPreserveComments(configPath, cfg); err != nil {
		t.Fatalf("SaveConfigPreserveComments() error = %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	out := string(data)
	for _, want := range []string{"api-key: object-key", "alias: Team Alias", "comment: Used by tests", "debug: true"} {
		if !strings.Contains(out, want) {
			t.Fatalf("saved config missing %q:\n%s", want, out)
		}
	}
}

func TestConfigJSONDoesNotExposeAPIKeyEntries(t *testing.T) {
	var cfg Config
	if err := yaml.Unmarshal([]byte(`api-keys:
  - api-key: object-key
    alias: Team Alias
    comment: Used by tests
`), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}
	data, err := json.Marshal(&cfg)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	jsonText := string(data)
	if strings.Contains(jsonText, "apiKeyEntries") || strings.Contains(jsonText, "api-key-entries") || strings.Contains(jsonText, "Team Alias") {
		t.Fatalf("JSON exposed companion metadata: %s", jsonText)
	}
	if !strings.Contains(jsonText, `"api-keys":["object-key"]`) {
		t.Fatalf("JSON did not preserve raw api-keys view: %s", jsonText)
	}
}
