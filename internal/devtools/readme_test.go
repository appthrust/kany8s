package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadmeDocumentsLocalDevLoop(t *testing.T) {
	root := findRepoRoot(t)

	readmePath := filepath.Join(root, "README.md")
	readmeBytes, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read %q: %v", readmePath, err)
	}

	readme := string(readmeBytes)
	wantSubstrings := []string{
		"## Development",
		"### Prerequisites",
		"### Quickstart",
		"`make test`",
		"`make lint`",
		"`make generate`",
		"`make manifests`",
		"`make run`",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(readme, want) {
			t.Errorf("README.md missing %q", want)
		}
	}
}
