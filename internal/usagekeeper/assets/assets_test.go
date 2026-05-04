package assets

import (
	"strings"
	"testing"
)

func TestPlaceholderAssetsIncludeIndexHTML(t *testing.T) {
	data, err := FS.ReadFile("dist/index.html")
	if err != nil {
		t.Fatalf("read embedded index.html: %v", err)
	}
	if !strings.Contains(string(data), "Usage Keeper") {
		t.Fatalf("embedded index.html does not identify usage keeper dashboard")
	}
}
