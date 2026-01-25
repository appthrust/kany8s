package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExamplesCAPIClusterSampleExists(t *testing.T) {
	root := findRepoRoot(t)

	examplePath := filepath.Join(root, "examples", "capi", "cluster.yaml")
	exampleBytes, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatalf("read %q: %v", examplePath, err)
	}

	example := string(exampleBytes)
	wantSubstrings := []string{
		"kind: Cluster",
		"apiVersion: cluster.x-k8s.io/v1beta2",
		"controlPlaneRef:",
		"apiVersion: controlplane.cluster.x-k8s.io/v1alpha1",
		"kind: Kany8sControlPlane",
		"kind: Kany8sControlPlane",
		"resourceGraphDefinitionRef:",
		"version:",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(example, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(examplePath), want)
		}
	}
}
