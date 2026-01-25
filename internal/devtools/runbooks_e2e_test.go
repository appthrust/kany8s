package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunbooksE2EExists(t *testing.T) {
	root := findRepoRoot(t)

	runbookPath := filepath.Join(root, "docs", "runbooks", "e2e.md")
	runbookBytes, err := os.ReadFile(runbookPath)
	if err != nil {
		t.Fatalf("read %q: %v", runbookPath, err)
	}

	runbook := string(runbookBytes)
	wantSubstrings := []string{
		"# E2E",
		"kubectl get clusters",
		"kubectl get kany8scontrolplanes",
		"kubectl describe kany8scontrolplane",
		"kubectl get resourcegraphdefinitions.kro.run",
		"kubectl get events -A",
		"kubectl logs -n kany8s-system deploy/kany8s-controller-manager",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(runbook, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(runbookPath), want)
		}
	}
}
