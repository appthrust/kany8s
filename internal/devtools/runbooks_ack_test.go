package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunbooksAckExists(t *testing.T) {
	root := findRepoRoot(t)

	runbookPath := filepath.Join(root, "docs", "runbooks", "ack.md")
	runbookBytes, err := os.ReadFile(runbookPath)
	if err != nil {
		t.Fatalf("read %q: %v", runbookPath, err)
	}

	runbook := string(runbookBytes)
	wantSubstrings := []string{
		"# ACK",
		"ack-system",
		"oci://public.ecr.aws/aws-controllers-k8s",
		"helm install",
		"kubectl create secret generic",
		"aws_access_key_id",
		"aws_secret_access_key",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(runbook, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(runbookPath), want)
		}
	}
}
