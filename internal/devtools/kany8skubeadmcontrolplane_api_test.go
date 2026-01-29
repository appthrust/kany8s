package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKany8sKubeadmControlPlaneSpecHasRequiredFields(t *testing.T) {
	root := findRepoRoot(t)

	typesPath := filepath.Join(root, "api", "v1alpha1", "kany8skubeadmcontrolplane_types.go")
	typesBytes, err := os.ReadFile(typesPath)
	if err != nil {
		t.Fatalf("read %q: %v", typesPath, err)
	}

	typesGo := string(typesBytes)
	wantSubstrings := []string{
		"Version string `json:\"version\"`",
		"Replicas *int32 `json:\"replicas,omitempty\"`",
		"MachineTemplate Kany8sKubeadmControlPlaneMachineTemplate `json:\"machineTemplate\"`",
		"InfrastructureRef clusterv1.ContractVersionedObjectReference `json:\"infrastructureRef\"`",
		"KubeadmConfigSpec *bootstrapv1.KubeadmConfigSpec `json:\"kubeadmConfigSpec,omitempty\"`",
		"ControlPlaneEndpoint clusterv1.APIEndpoint `json:\"controlPlaneEndpoint,omitempty\"`",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(typesGo, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(typesPath), want)
		}
	}
}

func TestGeneratedCRDBasesContainExpectedSchemaForKany8sKubeadmControlPlane(t *testing.T) {
	root := findRepoRoot(t)

	crdPath := filepath.Join(root, "config", "crd", "bases", "controlplane.cluster.x-k8s.io_kany8skubeadmcontrolplanes.yaml")
	crdBytes, err := os.ReadFile(crdPath)
	if err != nil {
		t.Fatalf("read %q: %v", crdPath, err)
	}

	crd := string(crdBytes)
	wantSubstrings := []string{
		"kind: CustomResourceDefinition",
		"name: kany8skubeadmcontrolplanes.controlplane.cluster.x-k8s.io",
		"group: controlplane.cluster.x-k8s.io",
		"kind: Kany8sKubeadmControlPlane",
		"subresources:",
		"status: {}",
		"controlPlaneEndpoint:",
		"kubeadmConfigSpec:",
		"machineTemplate:",
		"infrastructureRef:",
		"replicas:",
		"version:",
		"- machineTemplate",
		"- version",
		"- infrastructureRef",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(crd, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(crdPath), want)
		}
	}
}
