package devtools_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
)

const (
	infraClusterAPIVersion = "infrastructure.cluster.x-k8s.io/v1alpha1"
	kindKany8sCluster      = "Kany8sCluster"
	demoClusterName        = "demo-cluster"
	demoNamespace          = "default"
)

func TestKroInfraAcceptanceRGDManifestExists(t *testing.T) {
	root := findRepoRoot(t)

	dirPath := filepath.Join(root, "test", "acceptance_test", "manifests", "kro", "infra")
	info, err := os.Stat(dirPath)
	if err != nil {
		t.Fatalf("stat %q: %v", dirPath, err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", filepath.ToSlash(dirPath))
	}

	rgdPath := filepath.Join(root, "test", "acceptance_test", "manifests", "kro", "infra", "rgd.yaml")
	rgdBytes, err := os.ReadFile(rgdPath)
	if err != nil {
		t.Fatalf("read %q: %v", rgdPath, err)
	}

	rgd := string(rgdBytes)
	wantSubstrings := []string{
		"apiVersion: kro.run/v1alpha1",
		"kind: ResourceGraphDefinition",
		"name: demo-infra.kro.run",
		"kind: DemoInfrastructure",
		"clusterName:",
		"clusterNamespace:",
		"status:",
		"ready:",
		"reason:",
		"message:",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(rgd, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(rgdPath), want)
		}
	}
}

func TestKroInfraAcceptanceRGDManifestIsValidYAML(t *testing.T) {
	root := findRepoRoot(t)

	rgdPath := filepath.Join(root, "test", "acceptance_test", "manifests", "kro", "infra", "rgd.yaml")
	rgdBytes, err := os.ReadFile(rgdPath)
	if err != nil {
		t.Fatalf("read %q: %v", rgdPath, err)
	}

	jsonBytes, err := utilyaml.ToJSON(rgdBytes)
	if err != nil {
		t.Fatalf("parse %q as YAML: %v", rgdPath, err)
	}

	var obj unstructured.Unstructured
	if err := obj.UnmarshalJSON(jsonBytes); err != nil {
		t.Fatalf("decode %q into unstructured object: %v", rgdPath, err)
	}

	if got, want := obj.GetAPIVersion(), "kro.run/v1alpha1"; got != want {
		t.Fatalf("%s apiVersion=%q, want %q", filepath.ToSlash(rgdPath), got, want)
	}
	if got, want := obj.GetKind(), "ResourceGraphDefinition"; got != want {
		t.Fatalf("%s kind=%q, want %q", filepath.ToSlash(rgdPath), got, want)
	}
	if got, want := obj.GetName(), "demo-infra.kro.run"; got != want {
		t.Fatalf("%s metadata.name=%q, want %q", filepath.ToSlash(rgdPath), got, want)
	}
}

func TestKroInfraAcceptanceOwnerRefRGDManifestExists(t *testing.T) {
	root := findRepoRoot(t)

	rgdPath := filepath.Join(root, "test", "acceptance_test", "manifests", "kro", "infra", "rgd-ownerref.yaml")
	rgdBytes, err := os.ReadFile(rgdPath)
	if err != nil {
		t.Fatalf("read %q: %v", rgdPath, err)
	}

	rgd := string(rgdBytes)
	wantSubstrings := []string{
		"apiVersion: kro.run/v1alpha1",
		"kind: ResourceGraphDefinition",
		"name: demo-infra-ownerref.kro.run",
		"kind: DemoInfrastructureOwned",
		"clusterUID:",
		"includeWhen:",
		"${schema.spec.?clusterUID.orValue(\"\") != \"\"}",
		"cluster.x-k8s.io/cluster-name:",
		"ownerReferences:",
		"apiVersion: cluster.x-k8s.io/v1beta2",
		"kind: Cluster",
		"uid: ${schema.spec.?clusterUID.orValue(\"\")}",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(rgd, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(rgdPath), want)
		}
	}
}

func TestKroInfraAcceptanceOwnerRefRGDManifestIsValidYAML(t *testing.T) {
	root := findRepoRoot(t)

	rgdPath := filepath.Join(root, "test", "acceptance_test", "manifests", "kro", "infra", "rgd-ownerref.yaml")
	rgdBytes, err := os.ReadFile(rgdPath)
	if err != nil {
		t.Fatalf("read %q: %v", rgdPath, err)
	}

	jsonBytes, err := utilyaml.ToJSON(rgdBytes)
	if err != nil {
		t.Fatalf("parse %q as YAML: %v", rgdPath, err)
	}

	var obj unstructured.Unstructured
	if err := obj.UnmarshalJSON(jsonBytes); err != nil {
		t.Fatalf("decode %q into unstructured object: %v", rgdPath, err)
	}

	if got, want := obj.GetAPIVersion(), "kro.run/v1alpha1"; got != want {
		t.Fatalf("%s apiVersion=%q, want %q", filepath.ToSlash(rgdPath), got, want)
	}
	if got, want := obj.GetKind(), "ResourceGraphDefinition"; got != want {
		t.Fatalf("%s kind=%q, want %q", filepath.ToSlash(rgdPath), got, want)
	}
	if got, want := obj.GetName(), "demo-infra-ownerref.kro.run"; got != want {
		t.Fatalf("%s metadata.name=%q, want %q", filepath.ToSlash(rgdPath), got, want)
	}

	if got, found, err := unstructured.NestedString(obj.Object, "spec", "schema", "kind"); err != nil {
		t.Fatalf("%s get spec.schema.kind: %v", filepath.ToSlash(rgdPath), err)
	} else if !found {
		t.Fatalf("%s missing spec.schema.kind", filepath.ToSlash(rgdPath))
	} else if want := "DemoInfrastructureOwned"; got != want {
		t.Fatalf("%s spec.schema.kind=%q, want %q", filepath.ToSlash(rgdPath), got, want)
	}

	specMap, found, err := unstructured.NestedMap(obj.Object, "spec", "schema", "spec")
	if err != nil {
		t.Fatalf("%s get spec.schema.spec: %v", filepath.ToSlash(rgdPath), err)
	}
	if !found {
		t.Fatalf("%s missing spec.schema.spec", filepath.ToSlash(rgdPath))
	}
	for _, key := range []string{"clusterName", "clusterNamespace", "clusterUID"} {
		if _, ok := specMap[key]; !ok {
			t.Fatalf("%s missing spec.schema.spec.%s", filepath.ToSlash(rgdPath), key)
		}
	}
}

func TestKroInfraAcceptanceKany8sClusterTemplateExists(t *testing.T) {
	root := findRepoRoot(t)

	tplPath := filepath.Join(root, "test", "acceptance_test", "manifests", "kro", "kany8scluster.yaml.tpl")
	tplBytes, err := os.ReadFile(tplPath)
	if err != nil {
		t.Fatalf("read %q: %v", tplPath, err)
	}

	tpl := string(tplBytes)
	wantSubstrings := []string{
		"apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1",
		"kind: Kany8sCluster",
		"name: __CLUSTER_NAME__",
		"namespace: __NAMESPACE__",
		"resourceGraphDefinitionRef:",
		"name: __RGD_NAME__",
		"kroSpec:",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(tpl, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(tplPath), want)
		}
	}
}

func TestKroInfraAcceptanceKany8sClusterTemplateRendersToValidYAML(t *testing.T) {
	root := findRepoRoot(t)

	tplPath := filepath.Join(root, "test", "acceptance_test", "manifests", "kro", "kany8scluster.yaml.tpl")
	tplBytes, err := os.ReadFile(tplPath)
	if err != nil {
		t.Fatalf("read %q: %v", tplPath, err)
	}

	replacer := strings.NewReplacer(
		"__CLUSTER_NAME__", demoClusterName,
		"__NAMESPACE__", demoNamespace,
		"__RGD_NAME__", "demo-infra.kro.run",
	)
	rendered := replacer.Replace(string(tplBytes))
	for _, placeholder := range []string{"__CLUSTER_NAME__", "__NAMESPACE__", "__RGD_NAME__"} {
		if strings.Contains(rendered, placeholder) {
			t.Fatalf("%s rendered output still contains %q", filepath.ToSlash(tplPath), placeholder)
		}
	}

	jsonBytes, err := utilyaml.ToJSON([]byte(rendered))
	if err != nil {
		t.Fatalf("parse rendered template %q as YAML: %v", tplPath, err)
	}

	var obj unstructured.Unstructured
	if err := obj.UnmarshalJSON(jsonBytes); err != nil {
		t.Fatalf("decode rendered %q into unstructured object: %v", tplPath, err)
	}

	if got, want := obj.GetAPIVersion(), infraClusterAPIVersion; got != want {
		t.Fatalf("%s apiVersion=%q, want %q", filepath.ToSlash(tplPath), got, want)
	}
	if got, want := obj.GetKind(), kindKany8sCluster; got != want {
		t.Fatalf("%s kind=%q, want %q", filepath.ToSlash(tplPath), got, want)
	}
	if got, want := obj.GetName(), demoClusterName; got != want {
		t.Fatalf("%s metadata.name=%q, want %q", filepath.ToSlash(tplPath), got, want)
	}
	if got, want := obj.GetNamespace(), demoNamespace; got != want {
		t.Fatalf("%s metadata.namespace=%q, want %q", filepath.ToSlash(tplPath), got, want)
	}

	if got, found, err := unstructured.NestedString(obj.Object, "spec", "resourceGraphDefinitionRef", "name"); err != nil {
		t.Fatalf("%s get spec.resourceGraphDefinitionRef.name: %v", filepath.ToSlash(tplPath), err)
	} else if !found {
		t.Fatalf("%s missing spec.resourceGraphDefinitionRef.name", filepath.ToSlash(tplPath))
	} else if want := "demo-infra.kro.run"; got != want {
		t.Fatalf("%s spec.resourceGraphDefinitionRef.name=%q, want %q", filepath.ToSlash(tplPath), got, want)
	}

	if got, found, err := unstructured.NestedMap(obj.Object, "spec", "kroSpec"); err != nil {
		t.Fatalf("%s get spec.kroSpec: %v", filepath.ToSlash(tplPath), err)
	} else if !found {
		t.Fatalf("%s missing spec.kroSpec", filepath.ToSlash(tplPath))
	} else if got == nil {
		t.Fatalf("%s spec.kroSpec is nil, want object", filepath.ToSlash(tplPath))
	}
}

func TestKroInfraAcceptanceClusterTemplateExists(t *testing.T) {
	root := findRepoRoot(t)

	tplPath := filepath.Join(root, "test", "acceptance_test", "manifests", "kro", "infra", "cluster.yaml.tpl")
	tplBytes, err := os.ReadFile(tplPath)
	if err != nil {
		t.Fatalf("read %q: %v", tplPath, err)
	}

	tpl := string(tplBytes)
	wantSubstrings := []string{
		"apiVersion: cluster.x-k8s.io/v1beta2",
		"kind: Cluster",
		"name: __CLUSTER_NAME__",
		"namespace: __NAMESPACE__",
		"infrastructureRef:",
		"apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1",
		"kind: Kany8sCluster",
		"---",
		"resourceGraphDefinitionRef:",
		"name: __RGD_NAME__",
		"kroSpec:",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(tpl, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(tplPath), want)
		}
	}
}

func TestKroInfraAcceptanceClusterTemplateRendersToValidYAML(t *testing.T) {
	root := findRepoRoot(t)

	tplPath := filepath.Join(root, "test", "acceptance_test", "manifests", "kro", "infra", "cluster.yaml.tpl")
	tplBytes, err := os.ReadFile(tplPath)
	if err != nil {
		t.Fatalf("read %q: %v", tplPath, err)
	}

	replacer := strings.NewReplacer(
		"__CLUSTER_NAME__", demoClusterName,
		"__NAMESPACE__", demoNamespace,
		"__RGD_NAME__", "demo-infra-ownerref.kro.run",
	)
	rendered := replacer.Replace(string(tplBytes))
	for _, placeholder := range []string{"__CLUSTER_NAME__", "__NAMESPACE__", "__RGD_NAME__"} {
		if strings.Contains(rendered, placeholder) {
			t.Fatalf("%s rendered output still contains %q", filepath.ToSlash(tplPath), placeholder)
		}
	}

	objs := decodeUnstructuredYAMLDocuments(t, tplPath, rendered)
	if got, want := len(objs), 2; got != want {
		t.Fatalf("%s decoded %d objects, want %d", filepath.ToSlash(tplPath), got, want)
	}

	clusterObj := mustGetSingleObjectByKind(t, tplPath, objs, "Cluster")
	kany8sClusterObj := mustGetSingleObjectByKind(t, tplPath, objs, kindKany8sCluster)

	assertInfraAcceptanceClusterTemplateCluster(t, tplPath, clusterObj)
	assertInfraAcceptanceClusterTemplateKany8sCluster(t, tplPath, kany8sClusterObj)
}

func decodeUnstructuredYAMLDocuments(t *testing.T, filePath string, yamlText string) []unstructured.Unstructured {
	t.Helper()

	decoder := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(yamlText)), 4096)
	var objs []unstructured.Unstructured
	for {
		var obj unstructured.Unstructured
		err := decoder.Decode(&obj)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decode %s: %v", filepath.ToSlash(filePath), err)
		}
		if len(obj.Object) == 0 {
			continue
		}
		objs = append(objs, obj)
	}
	return objs
}

func mustGetSingleObjectByKind(t *testing.T, filePath string, objs []unstructured.Unstructured, kind string) *unstructured.Unstructured {
	t.Helper()

	var found *unstructured.Unstructured
	for i := range objs {
		obj := &objs[i]
		if obj.GetKind() != kind {
			continue
		}
		if found != nil {
			t.Fatalf("%s contains multiple objects with kind %q", filepath.ToSlash(filePath), kind)
		}
		found = obj
	}
	if found == nil {
		t.Fatalf("%s missing object with kind %q", filepath.ToSlash(filePath), kind)
	}
	return found
}

func mustNestedString(t *testing.T, filePath string, objKind string, obj map[string]any, fields ...string) string {
	t.Helper()

	got, found, err := unstructured.NestedString(obj, fields...)
	if err != nil {
		t.Fatalf("%s get %s %s: %v", filepath.ToSlash(filePath), objKind, strings.Join(fields, "."), err)
	}
	if !found {
		t.Fatalf("%s missing %s %s", filepath.ToSlash(filePath), objKind, strings.Join(fields, "."))
	}
	return got
}

func mustNestedMap(t *testing.T, filePath string, objKind string, obj map[string]any, fields ...string) map[string]any {
	t.Helper()

	got, found, err := unstructured.NestedMap(obj, fields...)
	if err != nil {
		t.Fatalf("%s get %s %s: %v", filepath.ToSlash(filePath), objKind, strings.Join(fields, "."), err)
	}
	if !found {
		t.Fatalf("%s missing %s %s", filepath.ToSlash(filePath), objKind, strings.Join(fields, "."))
	}
	if got == nil {
		t.Fatalf("%s %s %s is nil, want object", filepath.ToSlash(filePath), objKind, strings.Join(fields, "."))
	}
	return got
}

func assertInfraAcceptanceClusterTemplateCluster(t *testing.T, tplPath string, clusterObj *unstructured.Unstructured) {
	t.Helper()

	if got, want := clusterObj.GetAPIVersion(), "cluster.x-k8s.io/v1beta2"; got != want {
		t.Fatalf("%s Cluster apiVersion=%q, want %q", filepath.ToSlash(tplPath), got, want)
	}
	if got, want := clusterObj.GetName(), demoClusterName; got != want {
		t.Fatalf("%s Cluster metadata.name=%q, want %q", filepath.ToSlash(tplPath), got, want)
	}
	if got, want := clusterObj.GetNamespace(), demoNamespace; got != want {
		t.Fatalf("%s Cluster metadata.namespace=%q, want %q", filepath.ToSlash(tplPath), got, want)
	}
	if _, found, err := unstructured.NestedMap(clusterObj.Object, "spec", "controlPlaneRef"); err != nil {
		t.Fatalf("%s get Cluster spec.controlPlaneRef: %v", filepath.ToSlash(tplPath), err)
	} else if found {
		t.Fatalf("%s Cluster spec.controlPlaneRef must be omitted for this acceptance template", filepath.ToSlash(tplPath))
	}

	if got, want := mustNestedString(t, tplPath, "Cluster", clusterObj.Object, "spec", "infrastructureRef", "apiVersion"), infraClusterAPIVersion; got != want {
		t.Fatalf("%s Cluster spec.infrastructureRef.apiVersion=%q, want %q", filepath.ToSlash(tplPath), got, want)
	}
	if got, want := mustNestedString(t, tplPath, "Cluster", clusterObj.Object, "spec", "infrastructureRef", "kind"), kindKany8sCluster; got != want {
		t.Fatalf("%s Cluster spec.infrastructureRef.kind=%q, want %q", filepath.ToSlash(tplPath), got, want)
	}
	if got, want := mustNestedString(t, tplPath, "Cluster", clusterObj.Object, "spec", "infrastructureRef", "name"), demoClusterName; got != want {
		t.Fatalf("%s Cluster spec.infrastructureRef.name=%q, want %q", filepath.ToSlash(tplPath), got, want)
	}
	if got, want := mustNestedString(t, tplPath, "Cluster", clusterObj.Object, "spec", "infrastructureRef", "namespace"), demoNamespace; got != want {
		t.Fatalf("%s Cluster spec.infrastructureRef.namespace=%q, want %q", filepath.ToSlash(tplPath), got, want)
	}
}

func assertInfraAcceptanceClusterTemplateKany8sCluster(t *testing.T, tplPath string, kany8sClusterObj *unstructured.Unstructured) {
	t.Helper()

	if got, want := kany8sClusterObj.GetAPIVersion(), infraClusterAPIVersion; got != want {
		t.Fatalf("%s Kany8sCluster apiVersion=%q, want %q", filepath.ToSlash(tplPath), got, want)
	}
	if got, want := kany8sClusterObj.GetKind(), kindKany8sCluster; got != want {
		t.Fatalf("%s Kany8sCluster kind=%q, want %q", filepath.ToSlash(tplPath), got, want)
	}
	if got, want := kany8sClusterObj.GetName(), demoClusterName; got != want {
		t.Fatalf("%s Kany8sCluster metadata.name=%q, want %q", filepath.ToSlash(tplPath), got, want)
	}
	if got, want := kany8sClusterObj.GetNamespace(), demoNamespace; got != want {
		t.Fatalf("%s Kany8sCluster metadata.namespace=%q, want %q", filepath.ToSlash(tplPath), got, want)
	}

	if got, want := mustNestedString(t, tplPath, kindKany8sCluster, kany8sClusterObj.Object, "spec", "resourceGraphDefinitionRef", "name"), "demo-infra-ownerref.kro.run"; got != want {
		t.Fatalf("%s Kany8sCluster spec.resourceGraphDefinitionRef.name=%q, want %q", filepath.ToSlash(tplPath), got, want)
	}
	_ = mustNestedMap(t, tplPath, kindKany8sCluster, kany8sClusterObj.Object, "spec", "kroSpec")
}
