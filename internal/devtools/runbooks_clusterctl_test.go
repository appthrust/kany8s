package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunbooksClusterctlExists(t *testing.T) {
	root := findRepoRoot(t)

	runbookPath := filepath.Join(root, "docs", "runbooks", "clusterctl.md")
	runbookBytes, err := os.ReadFile(runbookPath)
	if err != nil {
		t.Fatalf("read %q: %v", runbookPath, err)
	}

	runbook := string(runbookBytes)
	wantSubstrings := []string{
		"# clusterctl",
		"make build-installer",
		"dist/install.yaml",
		"ControlPlaneProvider",
		"file://",
		"clusterctl init",
		"kany8s-system",
		"kubectl create namespace",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(runbook, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(runbookPath), want)
		}
	}
}
