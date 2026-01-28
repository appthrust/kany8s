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

func TestControlplaneKany8sClusterTemplateRemoved(t *testing.T) {
	root := findRepoRoot(t)

	typesPath := filepath.Join(root, "api", "v1alpha1", "kany8sclustertemplate_types.go")
	requireFileDoesNotExist(t, typesPath)

	crdBasePath := filepath.Join(root, "config", "crd", "bases", "controlplane.cluster.x-k8s.io_kany8sclustertemplates.yaml")
	requireFileDoesNotExist(t, crdBasePath)

	crdKustomizationPath := filepath.Join(root, "config", "crd", "kustomization.yaml")
	crdKustomizationBytes, err := os.ReadFile(crdKustomizationPath)
	if err != nil {
		t.Fatalf("read %q: %v", crdKustomizationPath, err)
	}
	if strings.Contains(string(crdKustomizationBytes), "bases/controlplane.cluster.x-k8s.io_kany8sclustertemplates.yaml") {
		t.Errorf("%s should not reference the removed controlplane Kany8sClusterTemplate CRD base", filepath.ToSlash(crdKustomizationPath))
	}

	samplePath := filepath.Join(root, "config", "samples", "controlplane_v1alpha1_kany8sclustertemplate.yaml")
	requireFileDoesNotExist(t, samplePath)

	samplesKustomizationPath := filepath.Join(root, "config", "samples", "kustomization.yaml")
	samplesKustomizationBytes, err := os.ReadFile(samplesKustomizationPath)
	if err != nil {
		t.Fatalf("read %q: %v", samplesKustomizationPath, err)
	}
	if strings.Contains(string(samplesKustomizationBytes), "controlplane_v1alpha1_kany8sclustertemplate.yaml") {
		t.Errorf("%s should not reference the removed controlplane Kany8sClusterTemplate sample", filepath.ToSlash(samplesKustomizationPath))
	}

	projectPath := filepath.Join(root, "PROJECT")
	projectBytes, err := os.ReadFile(projectPath)
	if err != nil {
		t.Fatalf("read %q: %v", projectPath, err)
	}
	compact := strings.NewReplacer(" ", "", "\t", "").Replace(string(projectBytes))
	if strings.Contains(compact, "kind:Kany8sClusterTemplate\npath:github.com/reoring/kany8s/api/v1alpha1") {
		t.Errorf("%s should not list the removed controlplane Kany8sClusterTemplate resource", filepath.ToSlash(projectPath))
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
		"Template Kany8sClusterTemplateResource",
		"type Kany8sClusterTemplateResource struct",
		"ObjectMeta clusterv1.ObjectMeta",
		"type Kany8sClusterTemplateResourceSpec struct",
		"KroSpec *apiextensionsv1.JSON",
		"type Kany8sClusterTemplate struct",
		"type Kany8sClusterTemplateList struct",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(typesGo, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(typesPath), want)
		}
	}

	if strings.Contains(typesGo, "Foo *string") {
		t.Errorf("%s should not define the scaffold example field Foo", filepath.ToSlash(typesPath))
	}
	if strings.Contains(typesGo, "+kubebuilder:subresource:status") {
		t.Errorf("%s should not enable the status subresource (template resources are not reconciled)", filepath.ToSlash(typesPath))
	}
	if strings.Contains(typesGo, "type Kany8sClusterTemplateStatus struct") {
		t.Errorf("%s should not define a status type (template resources are not reconciled)", filepath.ToSlash(typesPath))
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
		"template:",
		"kroSpec:",
		"- template",
	}
	for _, want := range wantInfraClusterTemplateSubstrings {
		if !strings.Contains(infraClusterTemplateCRD, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(infraClusterTemplateCRDPath), want)
		}
	}
	if strings.Contains(infraClusterTemplateCRD, "foo:") {
		t.Errorf("%s should not include the scaffold example spec field foo", filepath.ToSlash(infraClusterTemplateCRDPath))
	}
	if strings.Contains(infraClusterTemplateCRD, "subresources:") {
		t.Errorf("%s should not enable the status subresource for templates", filepath.ToSlash(infraClusterTemplateCRDPath))
	}
}

func requireFileDoesNotExist(t *testing.T, path string) {
	t.Helper()
	_, err := os.Stat(path)
	if err == nil {
		t.Fatalf("%s should not exist", filepath.ToSlash(path))
	}
	if !os.IsNotExist(err) {
		t.Fatalf("stat %q: %v", path, err)
	}
}
