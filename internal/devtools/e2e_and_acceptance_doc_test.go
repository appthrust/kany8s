package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2EAndAcceptanceDocListsKroInfraReflectionTargets(t *testing.T) {
	root := findRepoRoot(t)

	docPath := filepath.Join(root, "docs", "e2e-and-acceptance-test.md")
	docBytes, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read %q: %v", docPath, err)
	}

	doc := string(docBytes)
	wantSubstrings := []string{
		"make test-acceptance-kro-infra-reflection",
		"make test-acceptance-kro-infra-reflection-keep",
		"hack/acceptance-test-kro-infra-reflection.sh",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(doc, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(docPath), want)
		}
	}
}

func TestE2EAndAcceptanceDocListsKroInfraClusterIdentityTargets(t *testing.T) {
	root := findRepoRoot(t)

	docPath := filepath.Join(root, "docs", "e2e-and-acceptance-test.md")
	docBytes, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read %q: %v", docPath, err)
	}

	doc := string(docBytes)
	wantSubstrings := []string{
		"make test-acceptance-kro-infra-cluster-identity",
		"make test-acceptance-kro-infra-cluster-identity-keep",
		"hack/acceptance-test-kro-infra-cluster-identity.sh",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(doc, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(docPath), want)
		}
	}
}
