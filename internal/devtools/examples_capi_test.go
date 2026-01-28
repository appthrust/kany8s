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
		"infrastructureRef:",
		"apiGroup: infrastructure.cluster.x-k8s.io",
		"kind: Kany8sCluster",
		"controlPlaneRef:",
		"apiGroup: controlplane.cluster.x-k8s.io",
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

func TestExamplesCAPIClusterClassSampleExists(t *testing.T) {
	root := findRepoRoot(t)

	examplePath := filepath.Join(root, "examples", "capi", "clusterclass.yaml")
	exampleBytes, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatalf("read %q: %v", examplePath, err)
	}

	example := string(exampleBytes)
	wantSubstrings := []string{
		"kind: ClusterClass",
		"apiVersion: cluster.x-k8s.io/v1beta2",
		"apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1",
		"kind: Kany8sControlPlaneTemplate",
		"kind: Kany8sClusterTemplate",
		"variables:",
		"name: region",
		"name: vpc.subnetIDs",
		"name: vpc.securityGroupIDs",
		"patches:",
		"path: /spec/template/spec/kroSpec/region",
		"variable: region",
		"path: /spec/template/spec/kroSpec/vpc/subnetIDs",
		"variable: vpc.subnetIDs",
		"path: /spec/template/spec/kroSpec/vpc/securityGroupIDs",
		"variable: vpc.securityGroupIDs",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(example, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(examplePath), want)
		}
	}
}
