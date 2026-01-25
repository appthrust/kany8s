package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExamplesKroEKSControlPlaneRGDSampleExists(t *testing.T) {
	root := findRepoRoot(t)

	rgdPath := filepath.Join(root, "examples", "kro", "eks", "eks-control-plane-rgd.yaml")
	rgdBytes, err := os.ReadFile(rgdPath)
	if err != nil {
		t.Fatalf("read %q: %v", rgdPath, err)
	}

	rgd := string(rgdBytes)
	wantRGDSubstrings := []string{
		"apiVersion: kro.run/v1alpha1",
		"kind: ResourceGraphDefinition",
		"name: eks-control-plane.kro.run",
		"kind: EKSControlPlane",
		"status:",
		"endpoint: ${cluster",
		"ready: ${int(cluster",
		"readyWhen:",
		"${cluster.?status.?status.orValue(\"\") == \"ACTIVE\" && cluster.?status.?endpoint.orValue(\"\") != \"\"}",
		"== \"ACTIVE\"",
		"roleARN: ${clusterRole.status.ackResourceMetadata.arn}",
		"iam.services.k8s.aws/v1alpha1",
		"kind: Role",
		"eks.services.k8s.aws/v1alpha1",
		"kind: Cluster",
	}
	for _, want := range wantRGDSubstrings {
		if !strings.Contains(rgd, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(rgdPath), want)
		}
	}
}

func TestExamplesKroEKSAddonsRGDSampleExists(t *testing.T) {
	root := findRepoRoot(t)

	rgdPath := filepath.Join(root, "examples", "kro", "eks", "eks-addons-rgd.yaml")
	rgdBytes, err := os.ReadFile(rgdPath)
	if err != nil {
		t.Fatalf("read %q: %v", rgdPath, err)
	}

	rgd := string(rgdBytes)
	wantRGDSubstrings := []string{
		"apiVersion: kro.run/v1alpha1",
		"kind: ResourceGraphDefinition",
		"name: eks-addons.kro.run",
		"kind: EKSAddons",
		"kind: Addon",
		"ready:",
		"readyWhen:",
		"clusterName:",
		"coredns",
		"kube-proxy",
		"vpc-cni",
	}
	for _, want := range wantRGDSubstrings {
		if !strings.Contains(rgd, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(rgdPath), want)
		}
	}
}
