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

func TestGeneratedManagerRoleIncludesKroInstanceWildcardRBAC(t *testing.T) {
	root := findRepoRoot(t)

	rolePath := filepath.Join(root, "config", "rbac", "role.yaml")
	roleBytes, err := os.ReadFile(rolePath)
	if err != nil {
		t.Fatalf("read %q: %v", rolePath, err)
	}

	role := string(roleBytes)

	wantBlock := strings.Join([]string{
		"- apiGroups:\n  - kro.run\n  resources:\n  - '*'\n  verbs:\n  - create\n  - get\n  - list\n  - patch\n  - update\n  - watch\n",
	}, "")
	if !strings.Contains(role, wantBlock) {
		t.Errorf("%s missing expected kro wildcard RBAC rule block:\n%s", filepath.ToSlash(rolePath), wantBlock)
	}
}
