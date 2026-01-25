package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExamplesKroReadyEndpointSampleExists(t *testing.T) {
	root := findRepoRoot(t)

	rgdPath := filepath.Join(root, "examples", "kro", "ready-endpoint", "rgd.yaml")
	rgdBytes, err := os.ReadFile(rgdPath)
	if err != nil {
		t.Fatalf("read %q: %v", rgdPath, err)
	}

	rgd := string(rgdBytes)
	wantRGDSubstrings := []string{
		"apiVersion: kro.run/v1alpha1",
		"kind: ResourceGraphDefinition",
		"schema:",
		"apiVersion: v1alpha1",
		"kind: DemoControlPlane",
		"status:",
		"ready:",
		"int(",
		"availableReplicas",
		"endpoint:",
		"svc.cluster.local",
	}
	for _, want := range wantRGDSubstrings {
		if !strings.Contains(rgd, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(rgdPath), want)
		}
	}

	instancePath := filepath.Join(root, "examples", "kro", "ready-endpoint", "instance.yaml")
	instanceBytes, err := os.ReadFile(instancePath)
	if err != nil {
		t.Fatalf("read %q: %v", instancePath, err)
	}

	instance := string(instanceBytes)
	wantInstanceSubstrings := []string{
		"apiVersion: kro.run/v1alpha1",
		"kind: DemoControlPlane",
		"spec:",
		"name:",
		"version:",
	}
	for _, want := range wantInstanceSubstrings {
		if !strings.Contains(instance, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(instancePath), want)
		}
	}
}
