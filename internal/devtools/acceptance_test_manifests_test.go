package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKroInfraAcceptanceRGDManifestExists(t *testing.T) {
	root := findRepoRoot(t)

	rgdPath := filepath.Join(root, "test", "acceptance_test", "manifests", "kro", "infra", "rgd.yaml")
	rgdBytes, err := os.ReadFile(rgdPath)
	if err != nil {
		t.Fatalf("read %q: %v", rgdPath, err)
	}

	rgd := string(rgdBytes)
	wantSubstrings := []string{
		"apiVersion: kro.run/v1alpha1",
		"kind: ResourceGraphDefinition",
		"name: demo-infra.kro.run",
		"kind: DemoInfrastructure",
		"clusterName:",
		"clusterNamespace:",
		"status:",
		"ready:",
		"reason:",
		"message:",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(rgd, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(rgdPath), want)
		}
	}
}

func TestKroInfraAcceptanceKany8sClusterTemplateExists(t *testing.T) {
	root := findRepoRoot(t)

	tplPath := filepath.Join(root, "test", "acceptance_test", "manifests", "kro", "kany8scluster.yaml.tpl")
	tplBytes, err := os.ReadFile(tplPath)
	if err != nil {
		t.Fatalf("read %q: %v", tplPath, err)
	}

	tpl := string(tplBytes)
	wantSubstrings := []string{
		"apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1",
		"kind: Kany8sCluster",
		"name: __CLUSTER_NAME__",
		"namespace: __NAMESPACE__",
		"resourceGraphDefinitionRef:",
		"name: __RGD_NAME__",
		"kroSpec:",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(tpl, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(tplPath), want)
		}
	}
}
