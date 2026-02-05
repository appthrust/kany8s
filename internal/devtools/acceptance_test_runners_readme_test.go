package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcceptanceTestRunnersReadmeMentionsKroInfraReflectionRunner(t *testing.T) {
	root := findRepoRoot(t)

	readmePath := filepath.Join(root, "test", "acceptance_test", "README.md")
	readmeBytes, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read %q: %v", readmePath, err)
	}

	readme := string(readmeBytes)
	wantSubstrings := []string{
		"run-acceptance-kro-infra-reflection.sh",
		"hack/acceptance-test-kro-infra-reflection.sh",
		"Purpose: validate managed-kro infra \"status reflection\"",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(readme, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(readmePath), want)
		}
	}
}

func TestAcceptanceTestRunnersReadmeMentionsKroInfraClusterIdentityRunner(t *testing.T) {
	root := findRepoRoot(t)

	readmePath := filepath.Join(root, "test", "acceptance_test", "README.md")
	readmeBytes, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read %q: %v", readmePath, err)
	}

	readme := string(readmeBytes)
	wantSubstrings := []string{
		"run-acceptance-kro-infra-cluster-identity.sh",
		"hack/acceptance-test-kro-infra-cluster-identity.sh",
		"Purpose: validate managed-kro infra \"cluster identity\"",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(readme, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(readmePath), want)
		}
	}
}
