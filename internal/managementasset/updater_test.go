package managementasset

import (
	"os"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func TestDefaultManagementReleaseURLUsesMsinxPanel(t *testing.T) {
	t.Parallel()

	if config.DefaultPanelGitHubRepository != "https://github.com/msinx/Cli-Proxy-API-Management-Center" {
		t.Fatalf("DefaultPanelGitHubRepository = %q", config.DefaultPanelGitHubRepository)
	}
	if defaultManagementReleaseURL != "https://api.github.com/repos/msinx/Cli-Proxy-API-Management-Center/releases/latest" {
		t.Fatalf("defaultManagementReleaseURL = %q", defaultManagementReleaseURL)
	}
	if defaultManagementFallbackURL != "" {
		t.Fatalf("defaultManagementFallbackURL = %q, want disabled fallback", defaultManagementFallbackURL)
	}
}

func TestResolveReleaseURLUsesMsinxPanel(t *testing.T) {
	t.Parallel()

	want := "https://api.github.com/repos/msinx/Cli-Proxy-API-Management-Center/releases/latest"
	for _, input := range []string{
		"",
		"https://github.com/msinx/Cli-Proxy-API-Management-Center",
		"https://api.github.com/repos/msinx/Cli-Proxy-API-Management-Center/releases/latest",
	} {
		if got := resolveReleaseURL(input); got != want {
			t.Fatalf("resolveReleaseURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestConfigExampleUsesMsinxPanel(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../config.example.yaml")
	if err != nil {
		t.Fatalf("ReadFile(config.example.yaml) error = %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "msinx/Cli-Proxy-API-Management-Center") {
		t.Fatalf("config.example.yaml does not contain msinx panel repository")
	}
	if strings.Contains(text, "router-for-me/Cli-Proxy-API-Management-Center") {
		t.Fatalf("config.example.yaml still references official panel repository")
	}
}
