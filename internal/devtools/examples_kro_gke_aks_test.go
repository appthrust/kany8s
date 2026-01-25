package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExamplesKroGKEControlPlaneRGDTemplateExists(t *testing.T) {
	root := findRepoRoot(t)

	rgdPath := filepath.Join(root, "examples", "kro", "gke", "gke-control-plane-rgd.yaml")
	rgdBytes, err := os.ReadFile(rgdPath)
	if err != nil {
		t.Fatalf("read %q: %v", rgdPath, err)
	}

	rgd := string(rgdBytes)
	wantRGDSubstrings := []string{
		"apiVersion: kro.run/v1alpha1",
		"kind: ResourceGraphDefinition",
		"name: gke-control-plane.kro.run",
		"kind: GKEControlPlane",
		"status:",
		"endpoint:",
		"ready:",
		"int(",
		"container.cnrm.cloud.google.com/v1beta1",
		"kind: ContainerCluster",
		"readyWhen:",
	}
	for _, want := range wantRGDSubstrings {
		if !strings.Contains(rgd, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(rgdPath), want)
		}
	}
}

func TestExamplesKroAKSControlPlaneRGDTemplateExists(t *testing.T) {
	root := findRepoRoot(t)

	rgdPath := filepath.Join(root, "examples", "kro", "aks", "aks-control-plane-rgd.yaml")
	rgdBytes, err := os.ReadFile(rgdPath)
	if err != nil {
		t.Fatalf("read %q: %v", rgdPath, err)
	}

	rgd := string(rgdBytes)
	wantRGDSubstrings := []string{
		"apiVersion: kro.run/v1alpha1",
		"kind: ResourceGraphDefinition",
		"name: aks-control-plane.kro.run",
		"kind: AKSControlPlane",
		"status:",
		"endpoint:",
		"ready:",
		"int(",
		"containerservice.azure.com/",
		"kind: ManagedCluster",
		"readyWhen:",
		"provisioningState",
	}
	for _, want := range wantRGDSubstrings {
		if !strings.Contains(rgd, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(rgdPath), want)
		}
	}
}
