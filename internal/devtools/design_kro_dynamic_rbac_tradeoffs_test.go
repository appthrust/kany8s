package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesignDocExplainsKroDynamicRBACTradeoffs(t *testing.T) {
	root := findRepoRoot(t)

	designPath := filepath.Join(root, "docs", "adr", "0007-dynamic-gvk-rbac-tradeoffs.md")
	designBytes, err := os.ReadFile(designPath)
	if err != nil {
		t.Fatalf("read %q: %v", designPath, err)
	}

	design := string(designBytes)
	wantSubstrings := []string{
		"dynamic GVK",
		"kro.run",
		"resources=*",
		"future tightening approach",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(design, want) {
			t.Errorf("docs/adr/0007-dynamic-gvk-rbac-tradeoffs.md missing %q", want)
		}
	}
}
