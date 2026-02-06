package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSelfManagedAcceptanceTemplateUsesFacadeControlPlane(t *testing.T) {
	root := findRepoRoot(t)

	tplPath := filepath.Join(root, "test", "acceptance_test", "manifests", "self-managed-docker", "cluster.yaml.tpl")
	tplBytes, err := os.ReadFile(tplPath)
	if err != nil {
		t.Fatalf("read %q: %v", tplPath, err)
	}

	tpl := string(tplBytes)
	wantSubstrings := []string{
		"kind: Cluster",
		"controlPlaneRef:",
		"kind: Kany8sControlPlane",
		"kind: Kany8sControlPlane",
		"kubeadm:",
		"machineTemplate:",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(tpl, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(tplPath), want)
		}
	}
	if strings.Contains(tpl, "kind: Kany8sKubeadmControlPlane") {
		t.Errorf("%s should use facade Kany8sControlPlane in acceptance template", filepath.ToSlash(tplPath))
	}
}
