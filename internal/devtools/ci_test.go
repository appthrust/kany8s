package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCIWorkflowRunsTestsAndManifests(t *testing.T) {
	root := findRepoRoot(t)

	workflowPath := filepath.Join(root, ".github", "workflows", "ci.yaml")
	workflowBytes, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read %q: %v", workflowPath, err)
	}

	workflow := string(workflowBytes)
	wantSubstrings := []string{
		"pull_request",
		"push",
		"make test",
		"make manifests",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(workflow, want) {
			t.Errorf("ci workflow missing %q", want)
		}
	}
}
