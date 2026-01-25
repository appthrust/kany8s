package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunbooksKindKroExists(t *testing.T) {
	root := findRepoRoot(t)

	runbookPath := filepath.Join(root, "docs", "runbooks", "kind-kro.md")
	runbookBytes, err := os.ReadFile(runbookPath)
	if err != nil {
		t.Fatalf("read %q: %v", runbookPath, err)
	}

	runbook := string(runbookBytes)
	wantSubstrings := []string{
		"# kind + kro",
		"kind create cluster",
		"kro-core-install-manifests",
		"kubectl create namespace kro-system",
		"kubectl rollout status -n kro-system deploy/kro",
		"kind delete cluster",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(runbook, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(runbookPath), want)
		}
	}
}
