package assets

import (
	"strings"
	"testing"
)

func TestAssetsIncludeDashboardIndexHTML(t *testing.T) {
	data, err := FS.ReadFile("dist/index.html")
	if err != nil {
		t.Fatalf("read embedded index.html: %v", err)
	}
	if !strings.Contains(string(data), "CPA USAGE KEEPER") || !strings.Contains(string(data), "__APP_BASE_PATH__") {
		t.Fatalf("embedded index.html does not identify usage keeper dashboard")
	}
}
