package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInfrastructureResourceGraphDefinitionReferenceTypeExists(t *testing.T) {
	root := findRepoRoot(t)

	path := filepath.Join(root, "api", "infrastructure", "v1alpha1", "resourcegraphdefinitionreference_types.go")
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}

	goSrc := string(bytes)
	required := []string{
		"type ResourceGraphDefinitionReference struct",
		"+kubebuilder:validation:MinLength=1",
		"Name string `json:\"name\"`",
	}
	for _, want := range required {
		if !strings.Contains(goSrc, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(path), want)
		}
	}
}
