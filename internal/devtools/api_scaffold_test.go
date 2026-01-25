package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKany8sControlPlaneAPIScaffoldExists(t *testing.T) {
	root := findRepoRoot(t)

	typesPath := filepath.Join(root, "api", "v1alpha1", "kany8scontrolplane_types.go")
	typesBytes, err := os.ReadFile(typesPath)
	if err != nil {
		t.Fatalf("read %q: %v", typesPath, err)
	}

	typesGo := string(typesBytes)
	wantSubstrings := []string{
		"type Kany8sControlPlaneSpec struct",
		"type Kany8sControlPlane struct",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(typesGo, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(typesPath), want)
		}
	}
}
