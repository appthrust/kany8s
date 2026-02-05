package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseDocExists(t *testing.T) {
	root := findRepoRoot(t)

	docPath := filepath.Join(root, "docs", "runbooks", "release.md")
	docBytes, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read %q: %v", docPath, err)
	}

	doc := string(docBytes)
	wantSubstrings := []string{
		"# Release",
		"## Versioning",
		"SemVer",
		"git tag",
		"make build-installer",
		"dist/install.yaml",
		"make docker-build",
		"make docker-push",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(doc, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(docPath), want)
		}
	}
}
