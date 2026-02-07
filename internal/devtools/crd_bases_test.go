package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratedCRDBasesContainExpectedSchema(t *testing.T) {
	root := findRepoRoot(t)

	crdPath := filepath.Join(root, "config", "crd", "bases", "controlplane.cluster.x-k8s.io_kany8scontrolplanes.yaml")
	crdBytes, err := os.ReadFile(crdPath)
	if err != nil {
		t.Fatalf("read %q: %v", crdPath, err)
	}

	crd := string(crdBytes)
	wantSubstrings := []string{
		"kind: CustomResourceDefinition",
		"name: kany8scontrolplanes.controlplane.cluster.x-k8s.io",
		"additionalPrinterColumns:",
		"name: INITIALIZED",
		"jsonPath: .status.initialization.controlPlaneInitialized",
		"name: ENDPOINT",
		"jsonPath: .spec.controlPlaneEndpoint.host",
		"x-kubernetes-preserve-unknown-fields: true",
		"subresources:",
		"status: {}",
		"resourceGraphDefinitionRef:",
		"kubeadm:",
		"externalBackend:",
		"- version",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(crd, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(crdPath), want)
		}
	}
	if strings.Contains(crd, "- resourceGraphDefinitionRef") {
		t.Errorf("%s should not require spec.resourceGraphDefinitionRef (backend selector is optional per field; webhook enforces exactly one)", filepath.ToSlash(crdPath))
	}
}
