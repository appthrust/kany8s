package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesignDocDefinesProviderAgnosticKubeconfigContract(t *testing.T) {
	root := findRepoRoot(t)

	designPath := filepath.Join(root, "docs", "adr", "0004-kubeconfig-secret-strategy.md")
	designBytes, err := os.ReadFile(designPath)
	if err != nil {
		t.Fatalf("read %q: %v", designPath, err)
	}

	design := string(designBytes)
	wantSubstrings := []string{
		"kubeconfigSecretRef",
		"status.kubeconfigSecretRef",
		"data.value",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(design, want) {
			t.Errorf("docs/adr/0004-kubeconfig-secret-strategy.md missing %q", want)
		}
	}
}
