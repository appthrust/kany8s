package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExamplesSelfManagedDockerSampleExists(t *testing.T) {
	root := findRepoRoot(t)

	examplePath := filepath.Join(root, "examples", "self-managed-docker", "cluster.yaml")
	exampleBytes, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatalf("read %q: %v", examplePath, err)
	}

	example := string(exampleBytes)
	wantSubstrings := []string{
		"kind: Cluster",
		"apiVersion: cluster.x-k8s.io/v1beta2",
		"infrastructureRef:",
		"apiGroup: infrastructure.cluster.x-k8s.io",
		"kind: DockerCluster",
		"controlPlaneRef:",
		"apiGroup: controlplane.cluster.x-k8s.io",
		"kind: Kany8sKubeadmControlPlane",
		"apiVersion: infrastructure.cluster.x-k8s.io/v1beta2",
		"kind: DockerMachineTemplate",
		"apiVersion: controlplane.cluster.x-k8s.io/v1alpha1",
		"machineTemplate:",
		"infrastructureRef:",
		"version:",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(example, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(examplePath), want)
		}
	}
}
