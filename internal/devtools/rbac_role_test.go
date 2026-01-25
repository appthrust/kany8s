package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratedManagerRoleIncludesResourceGraphDefinitionReadRBAC(t *testing.T) {
	root := findRepoRoot(t)

	rolePath := filepath.Join(root, "config", "rbac", "role.yaml")
	roleBytes, err := os.ReadFile(rolePath)
	if err != nil {
		t.Fatalf("read %q: %v", rolePath, err)
	}

	role := string(roleBytes)
	wantSubstrings := []string{
		"apiGroups:\n  - kro.run",
		"resources:\n  - resourcegraphdefinitions",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(role, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(rolePath), want)
		}
	}
}
