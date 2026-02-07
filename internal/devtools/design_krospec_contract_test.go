package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesignDocClarifiesKroSpecMustBeJSONObject(t *testing.T) {
	root := findRepoRoot(t)

	designPath := filepath.Join(root, "docs", "adr", "0003-kro-instance-lifecycle-and-spec-injection.md")
	designBytes, err := os.ReadFile(designPath)
	if err != nil {
		t.Fatalf("read %q: %v", designPath, err)
	}

	design := string(designBytes)
	wantSubstrings := []string{
		"`spec.kroSpec` must be a JSON object",
		"injects `spec.version`",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(design, want) {
			t.Errorf("docs/adr/0003-kro-instance-lifecycle-and-spec-injection.md missing %q", want)
		}
	}
}
