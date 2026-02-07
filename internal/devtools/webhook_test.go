package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestControlPlaneWebhookManifestsGenerated(t *testing.T) {
	root := findRepoRoot(t)

	manifestPath := filepath.Join(root, "config", "webhook", "manifests.yaml")
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read %q: %v", manifestPath, err)
	}
	manifest := string(manifestBytes)

	wantSubstrings := []string{
		"kind: ValidatingWebhookConfiguration",
		"name: validating-webhook-configuration",
		"/validate-controlplane-cluster-x-k8s-io-v1alpha1-kany8scontrolplane",
		"/validate-controlplane-cluster-x-k8s-io-v1alpha1-kany8scontrolplanetemplate",
		"vkany8scontrolplane.kb.io",
		"vkany8scontrolplanetemplate.kb.io",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(manifest, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(manifestPath), want)
		}
	}
}

func TestMainRegistersControlPlaneWebhooks(t *testing.T) {
	root := findRepoRoot(t)

	mainPath := filepath.Join(root, "cmd", "main.go")
	mainBytes, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("read %q: %v", mainPath, err)
	}
	mainGo := string(mainBytes)

	wantSubstrings := []string{
		"(&controlplanev1alpha1.Kany8sControlPlane{}).SetupWebhookWithManager(mgr)",
		"(&controlplanev1alpha1.Kany8sControlPlaneTemplate{}).SetupWebhookWithManager(mgr)",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(mainGo, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(mainPath), want)
		}
	}
}
