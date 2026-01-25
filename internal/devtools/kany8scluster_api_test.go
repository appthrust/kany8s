package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKany8sClusterAPIScaffoldExists(t *testing.T) {
	root := findRepoRoot(t)

	typesPath := filepath.Join(root, "api", "infrastructure", "v1alpha1", "kany8scluster_types.go")
	typesBytes, err := os.ReadFile(typesPath)
	if err != nil {
		t.Fatalf("read %q: %v", typesPath, err)
	}

	typesGo := string(typesBytes)
	wantSubstrings := []string{
		"type Kany8sClusterSpec struct",
		"type Kany8sClusterStatus struct",
		"type Kany8sCluster struct",
		"KroSpec *apiextensionsv1.JSON",
		"Conditions []metav1.Condition",
		"// +kubebuilder:subresource:status",
		"cluster.x-k8s.io/v1beta2=v1alpha1",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(typesGo, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(typesPath), want)
		}
	}

	if strings.Contains(typesGo, "Foo *string") {
		t.Errorf("%s should not include the scaffold example field Foo", filepath.ToSlash(typesPath))
	}
}

func TestGeneratedCRDBasesContainExpectedSchemaForKany8sCluster(t *testing.T) {
	root := findRepoRoot(t)

	crdPath := filepath.Join(root, "config", "crd", "bases", "infrastructure.cluster.x-k8s.io_kany8sclusters.yaml")
	crdBytes, err := os.ReadFile(crdPath)
	if err != nil {
		t.Fatalf("read %q: %v", crdPath, err)
	}

	crd := string(crdBytes)
	wantSubstrings := []string{
		"kind: CustomResourceDefinition",
		"cluster.x-k8s.io/v1beta2: v1alpha1",
		"name: kany8sclusters.infrastructure.cluster.x-k8s.io",
		"group: infrastructure.cluster.x-k8s.io",
		"kind: Kany8sCluster",
		"plural: kany8sclusters",
		"subresources:",
		"status: {}",
		"x-kubernetes-preserve-unknown-fields: true",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(crd, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(crdPath), want)
		}
	}
}
