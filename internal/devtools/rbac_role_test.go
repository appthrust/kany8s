package devtools_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

type rbacRole struct {
	Rules []rbacPolicyRule `yaml:"rules"`
}

type rbacPolicyRule struct {
	APIGroups []string `yaml:"apiGroups"`
	Resources []string `yaml:"resources"`
	Verbs     []string `yaml:"verbs"`
}

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

func TestGeneratedManagerRoleIncludesEventsRBAC(t *testing.T) {
	root := findRepoRoot(t)

	rolePath := filepath.Join(root, "config", "rbac", "role.yaml")
	roleBytes, err := os.ReadFile(rolePath)
	if err != nil {
		t.Fatalf("read %q: %v", rolePath, err)
	}

	var role rbacRole
	if err := yaml.Unmarshal(roleBytes, &role); err != nil {
		t.Fatalf("parse %q: %v", rolePath, err)
	}

	for _, group := range []string{"", "events.k8s.io"} {
		found := false
		for _, rule := range role.Rules {
			if !slices.Contains(rule.APIGroups, group) {
				continue
			}
			if !slices.Contains(rule.Resources, "events") {
				continue
			}
			if !slices.Contains(rule.Verbs, "create") || !slices.Contains(rule.Verbs, "patch") {
				continue
			}
			found = true
			break
		}
		if !found {
			t.Errorf("%s missing create/patch RBAC for events apiGroup %q", filepath.ToSlash(rolePath), group)
		}
	}
}

func TestGeneratedManagerRoleIncludesSecretsRBAC(t *testing.T) {
	root := findRepoRoot(t)

	rolePath := filepath.Join(root, "config", "rbac", "role.yaml")
	roleBytes, err := os.ReadFile(rolePath)
	if err != nil {
		t.Fatalf("read %q: %v", rolePath, err)
	}

	var role rbacRole
	if err := yaml.Unmarshal(roleBytes, &role); err != nil {
		t.Fatalf("parse %q: %v", rolePath, err)
	}

	foundRequired := false
	for _, rule := range role.Rules {
		if !slices.Contains(rule.APIGroups, "") {
			continue
		}
		if !slices.Contains(rule.Resources, "secrets") {
			continue
		}

		for _, disallowed := range []string{"list", "watch"} {
			if slices.Contains(rule.Verbs, disallowed) {
				t.Errorf("%s secrets RBAC should not include verb %q", filepath.ToSlash(rolePath), disallowed)
			}
		}

		missing := []string{}
		for _, verb := range []string{"create", "get", "patch", "update"} {
			if !slices.Contains(rule.Verbs, verb) {
				missing = append(missing, verb)
			}
		}
		if len(missing) == 0 {
			foundRequired = true
		}
	}
	if !foundRequired {
		t.Errorf("%s missing RBAC rule for core secrets with create/get/update/patch", filepath.ToSlash(rolePath))
	}
}

func TestGeneratedManagerRoleIncludesKany8sControlPlaneRBAC(t *testing.T) {
	root := findRepoRoot(t)

	rolePath := filepath.Join(root, "config", "rbac", "role.yaml")
	roleBytes, err := os.ReadFile(rolePath)
	if err != nil {
		t.Fatalf("read %q: %v", rolePath, err)
	}

	var role rbacRole
	if err := yaml.Unmarshal(roleBytes, &role); err != nil {
		t.Fatalf("parse %q: %v", rolePath, err)
	}

	requireRule := func(apiGroup, resource string, verbs ...string) {
		t.Helper()

		for _, rule := range role.Rules {
			if !slices.Contains(rule.APIGroups, apiGroup) {
				continue
			}
			if !slices.Contains(rule.Resources, resource) {
				continue
			}
			missing := []string{}
			for _, verb := range verbs {
				if !slices.Contains(rule.Verbs, verb) {
					missing = append(missing, verb)
				}
			}
			if len(missing) == 0 {
				return
			}
		}

		t.Errorf("%s missing RBAC rule for %s %s with verbs %s", filepath.ToSlash(rolePath), apiGroup, resource, strings.Join(verbs, ","))
	}

	requireRule(
		"controlplane.cluster.x-k8s.io",
		"kany8scontrolplanes",
		"create",
		"delete",
		"get",
		"list",
		"patch",
		"update",
		"watch",
	)
	requireRule(
		"controlplane.cluster.x-k8s.io",
		"kany8scontrolplanes/status",
		"get",
		"patch",
		"update",
	)
	requireRule(
		"controlplane.cluster.x-k8s.io",
		"kany8scontrolplanes/finalizers",
		"update",
	)
}
