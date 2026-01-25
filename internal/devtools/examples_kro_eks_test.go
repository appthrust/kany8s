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
		"awsAccountID:",
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
