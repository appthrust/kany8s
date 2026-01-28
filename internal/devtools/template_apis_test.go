package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKany8sControlPlaneTemplateAPIScaffoldExists(t *testing.T) {
	root := findRepoRoot(t)

	typesPath := filepath.Join(root, "api", "v1alpha1", "kany8scontrolplanetemplate_types.go")
	typesBytes, err := os.ReadFile(typesPath)
	if err != nil {
		t.Fatalf("read %q: %v", typesPath, err)
	}

	typesGo := string(typesBytes)
	wantSubstrings := []string{
		"type Kany8sControlPlaneTemplateSpec struct",
		"type Kany8sControlPlaneTemplate struct",
		"type Kany8sControlPlaneTemplateResource struct",
		"ResourceGraphDefinitionRef ResourceGraphDefinitionReference",
		"KroSpec *apiextensionsv1.JSON",
		"ObjectMeta clusterv1.ObjectMeta",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(typesGo, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(typesPath), want)
		}
	}

	if strings.Contains(typesGo, "Version string `json:\"version\"`") {
		t.Errorf("%s should not define spec.template.spec.version (topology-controlled)", filepath.ToSlash(typesPath))
	}
	if strings.Contains(typesGo, "ControlPlaneEndpoint") {
		t.Errorf("%s should not define spec.template.spec.controlPlaneEndpoint (controller-controlled)", filepath.ToSlash(typesPath))
	}
}

func TestKany8sClusterTemplateAPIScaffoldExists(t *testing.T) {
	root := findRepoRoot(t)

	typesPath := filepath.Join(root, "api", "v1alpha1", "kany8sclustertemplate_types.go")
	typesBytes, err := os.ReadFile(typesPath)
	if err != nil {
		t.Fatalf("read %q: %v", typesPath, err)
	}

	typesGo := string(typesBytes)
	wantSubstrings := []string{
		"type Kany8sClusterTemplateSpec struct",
		"type Kany8sClusterTemplate struct",
		"type Kany8sClusterTemplateResource struct",
		"ObjectMeta clusterv1.ObjectMeta",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(typesGo, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(typesPath), want)
		}
	}
}

func TestInfrastructureKany8sClusterTemplateAPIScaffoldExists(t *testing.T) {
	root := findRepoRoot(t)

	typesPath := filepath.Join(root, "api", "infrastructure", "v1alpha1", "kany8sclustertemplate_types.go")
	typesBytes, err := os.ReadFile(typesPath)
	if err != nil {
		t.Fatalf("read %q: %v", typesPath, err)
	}

	typesGo := string(typesBytes)
	wantSubstrings := []string{
		"type Kany8sClusterTemplateSpec struct",
		"type Kany8sClusterTemplate struct",
		"type Kany8sClusterTemplateList struct",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(typesGo, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(typesPath), want)
		}
	}
}

func TestGeneratedCRDBasesContainExpectedSchemaForTemplates(t *testing.T) {
	root := findRepoRoot(t)

	controlPlaneTemplateCRDPath := filepath.Join(root, "config", "crd", "bases", "controlplane.cluster.x-k8s.io_kany8scontrolplanetemplates.yaml")
	controlPlaneTemplateCRDBytes, err := os.ReadFile(controlPlaneTemplateCRDPath)
	if err != nil {
		t.Fatalf("read %q: %v", controlPlaneTemplateCRDPath, err)
	}
	controlPlaneTemplateCRD := string(controlPlaneTemplateCRDBytes)
	wantControlPlaneTemplateSubstrings := []string{
		"kind: CustomResourceDefinition",
		"name: kany8scontrolplanetemplates.controlplane.cluster.x-k8s.io",
		"kind: Kany8sControlPlaneTemplate",
		"plural: kany8scontrolplanetemplates",
		"resourceGraphDefinitionRef:",
		"kroSpec:",
		"- resourceGraphDefinitionRef",
		"- template",
	}
	for _, want := range wantControlPlaneTemplateSubstrings {
		if !strings.Contains(controlPlaneTemplateCRD, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(controlPlaneTemplateCRDPath), want)
		}
	}
	if strings.Contains(controlPlaneTemplateCRD, "- version") {
		t.Errorf("%s should not require spec.template.spec.version (topology-controlled)", filepath.ToSlash(controlPlaneTemplateCRDPath))
	}

	clusterTemplateCRDPath := filepath.Join(root, "config", "crd", "bases", "controlplane.cluster.x-k8s.io_kany8sclustertemplates.yaml")
	clusterTemplateCRDBytes, err := os.ReadFile(clusterTemplateCRDPath)
	if err != nil {
		t.Fatalf("read %q: %v", clusterTemplateCRDPath, err)
	}
	clusterTemplateCRD := string(clusterTemplateCRDBytes)
	wantClusterTemplateSubstrings := []string{
		"kind: CustomResourceDefinition",
		"name: kany8sclustertemplates.controlplane.cluster.x-k8s.io",
		"kind: Kany8sClusterTemplate",
		"plural: kany8sclustertemplates",
		"- template",
	}
	for _, want := range wantClusterTemplateSubstrings {
		if !strings.Contains(clusterTemplateCRD, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(clusterTemplateCRDPath), want)
		}
	}

	infraClusterTemplateCRDPath := filepath.Join(root, "config", "crd", "bases", "infrastructure.cluster.x-k8s.io_kany8sclustertemplates.yaml")
	infraClusterTemplateCRDBytes, err := os.ReadFile(infraClusterTemplateCRDPath)
	if err != nil {
		t.Fatalf("read %q: %v", infraClusterTemplateCRDPath, err)
	}
	infraClusterTemplateCRD := string(infraClusterTemplateCRDBytes)
	wantInfraClusterTemplateSubstrings := []string{
		"kind: CustomResourceDefinition",
		"name: kany8sclustertemplates.infrastructure.cluster.x-k8s.io",
		"kind: Kany8sClusterTemplate",
		"plural: kany8sclustertemplates",
	}
	for _, want := range wantInfraClusterTemplateSubstrings {
		if !strings.Contains(infraClusterTemplateCRD, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(infraClusterTemplateCRDPath), want)
		}
	}
}
