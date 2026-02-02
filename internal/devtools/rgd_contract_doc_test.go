package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRgdContractDocExists(t *testing.T) {
	root := findRepoRoot(t)

	docPath := filepath.Join(root, "docs", "rgd-contract.md")
	docBytes, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read %q: %v", docPath, err)
	}

	doc := string(docBytes)
	wantSubstrings := []string{
		"# RGD Contract",
		"`Kany8sCluster`",
		"`status.ready`",
		"`status.endpoint`",
		"`status.reason`",
		"`status.message`",
		"Infrastructure",
		"`status.initialization.provisioned`",
		"RGD authors",
		"tolerates missing",
		"docs/rgd-guidelines.md",
		"https://host[:port]",
		"host[:port]",
		"443",
		"status.conditions",
		"status.state",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(doc, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(docPath), want)
		}
	}
}
