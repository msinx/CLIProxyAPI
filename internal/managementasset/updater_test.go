package managementasset

import "testing"

func TestResolveReleaseURLDefaultsToPersonalRepo(t *testing.T) {
	got := resolveReleaseURL("")
	want := "https://api.github.com/repos/msinx/Cli-Proxy-API-Management-Center/releases/latest"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestDefaultManagementFallbackURLUnchanged(t *testing.T) {
	want := "https://cpamc.router-for.me/"
	if defaultManagementFallbackURL != want {
		t.Fatalf("fallback URL changed to %q, want %q", defaultManagementFallbackURL, want)
	}
}
