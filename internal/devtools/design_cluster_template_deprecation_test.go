package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesignDocDeprecatesControlplaneKany8sClusterTemplate(t *testing.T) {
	root := findRepoRoot(t)

	designPath := filepath.Join(root, "docs", "adr", "0006-topology-version-and-template-api-groups.md")
	designBytes, err := os.ReadFile(designPath)
	if err != nil {
		t.Fatalf("read %q: %v", designPath, err)
	}

	design := string(designBytes)
	wantSubstrings := []string{
		"Kany8sClusterTemplate",
		"infrastructure.cluster.x-k8s.io",
		"controlplane.cluster.x-k8s.io",
		"removed",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(design, want) {
			t.Errorf("docs/adr/0006-topology-version-and-template-api-groups.md missing %q", want)
		}
	}
}
