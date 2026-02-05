package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesignDocDefinesTopologyVersionSingleSourceOfTruth(t *testing.T) {
	root := findRepoRoot(t)

	designPath := filepath.Join(root, "docs", "adr", "0006-topology-version-and-template-api-groups.md")
	designBytes, err := os.ReadFile(designPath)
	if err != nil {
		t.Fatalf("read %q: %v", designPath, err)
	}

	design := string(designBytes)
	wantSubstrings := []string{
		"Cluster.spec.topology.version",
		"Kany8sControlPlane.spec.version",
		"single source of truth",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(design, want) {
			t.Errorf("docs/adr/0006-topology-version-and-template-api-groups.md missing %q", want)
		}
	}
}
