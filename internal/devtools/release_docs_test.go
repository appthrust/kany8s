package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseDocsExists(t *testing.T) {
	root := findRepoRoot(t)

	releasePath := filepath.Join(root, "docs", "release.md")
	releaseBytes, err := os.ReadFile(releasePath)
	if err != nil {
		t.Fatalf("read %q: %v", releasePath, err)
	}

	release := string(releaseBytes)
	wantSubstrings := []string{
		"# Release",
		"## Versioning",
		"## Release Process",
		"git tag",
		"gh release",
		"make docker-build",
		"make docker-push",
		"make build-installer",
		"dist/install.yaml",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(release, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(releasePath), want)
		}
	}
}
