package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodebaseDocListsKroInfraReflectionAcceptanceEntryPoints(t *testing.T) {
	root := findRepoRoot(t)

	docPath := filepath.Join(root, "docs", "reference", "codebase.md")
	docBytes, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read %q: %v", docPath, err)
	}

	doc := string(docBytes)
	wantSubstrings := []string{
		"make test-acceptance-kro-infra-reflection",
		"make test-acceptance-kro-infra-reflection-keep",
		"hack/acceptance-test-kro-infra-reflection.sh",
		"test/acceptance_test/run-acceptance-kro-infra-reflection.sh",
		"test/acceptance_test/manifests/kro/infra/rgd.yaml",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(doc, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(docPath), want)
		}
	}
}

func TestCodebaseDocListsKroInfraClusterIdentityAcceptanceEntryPoints(t *testing.T) {
	root := findRepoRoot(t)

	docPath := filepath.Join(root, "docs", "reference", "codebase.md")
	docBytes, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read %q: %v", docPath, err)
	}

	doc := string(docBytes)
	wantSubstrings := []string{
		"make test-acceptance-kro-infra-cluster-identity",
		"make test-acceptance-kro-infra-cluster-identity-keep",
		"hack/acceptance-test-kro-infra-cluster-identity.sh",
		"test/acceptance_test/run-acceptance-kro-infra-cluster-identity.sh",
		"test/acceptance_test/manifests/kro/infra/rgd-ownerref.yaml",
		"test/acceptance_test/manifests/kro/infra/cluster.yaml.tpl",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(doc, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(docPath), want)
		}
	}
}
