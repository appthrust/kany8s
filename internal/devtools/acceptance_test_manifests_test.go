package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
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

func TestKroInfraAcceptanceRGDManifestIsValidYAML(t *testing.T) {
	root := findRepoRoot(t)

	rgdPath := filepath.Join(root, "test", "acceptance_test", "manifests", "kro", "infra", "rgd.yaml")
	rgdBytes, err := os.ReadFile(rgdPath)
	if err != nil {
		t.Fatalf("read %q: %v", rgdPath, err)
	}

	jsonBytes, err := utilyaml.ToJSON(rgdBytes)
	if err != nil {
		t.Fatalf("parse %q as YAML: %v", rgdPath, err)
	}

	var obj unstructured.Unstructured
	if err := obj.UnmarshalJSON(jsonBytes); err != nil {
		t.Fatalf("decode %q into unstructured object: %v", rgdPath, err)
	}

	if got, want := obj.GetAPIVersion(), "kro.run/v1alpha1"; got != want {
		t.Fatalf("%s apiVersion=%q, want %q", filepath.ToSlash(rgdPath), got, want)
	}
	if got, want := obj.GetKind(), "ResourceGraphDefinition"; got != want {
		t.Fatalf("%s kind=%q, want %q", filepath.ToSlash(rgdPath), got, want)
	}
	if got, want := obj.GetName(), "demo-infra.kro.run"; got != want {
		t.Fatalf("%s metadata.name=%q, want %q", filepath.ToSlash(rgdPath), got, want)
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
