//go:build releaseartifact

package releaseartifact_test

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"k8s.io/apimachinery/pkg/util/yaml"
)

const contractLabel = "cluster.x-k8s.io/v1beta2"

var expectedCRDs = map[string][]string{
	"infrastructure-components.yaml": {
		"kany8sclusters.infrastructure.cluster.x-k8s.io",
		"kany8sclustertemplates.infrastructure.cluster.x-k8s.io",
	},
	"control-plane-components.yaml": {
		"kany8scontrolplanes.controlplane.cluster.x-k8s.io",
		"kany8scontrolplanetemplates.controlplane.cluster.x-k8s.io",
		"kany8skubeadmcontrolplanes.controlplane.cluster.x-k8s.io",
	},
}

func TestClusterctlArtifactsCarryCAPIContractLabels(t *testing.T) {
	root := repoRoot(t)
	name := envDefault("CLUSTERCTL_NAME", "kany8s")
	version := envDefault("CLUSTERCTL_PROVIDER_VERSION", "v0.0.0")

	artifactPaths := map[string]string{
		"infrastructure-components.yaml": filepath.Join(root, "out", "infrastructure-"+name, version, "infrastructure-components.yaml"),
		"control-plane-components.yaml":  filepath.Join(root, "out", "control-plane-"+name, version, "control-plane-components.yaml"),
	}

	for artifact, crdNames := range expectedCRDs {
		t.Run(artifact, func(t *testing.T) {
			crds, err := readCRDs(artifactPaths[artifact])
			if err != nil {
				t.Fatal(err)
			}
			for _, crdName := range crdNames {
				doc, ok := crds[crdName]
				if !ok {
					t.Fatalf("missing CRD %s in %s", crdName, artifactPaths[artifact])
				}
				assertContractLabel(t, crdName, doc)
				assertV1Beta2ServedAlias(t, crdName, doc)
			}
		})
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve test file path")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repository root containing go.mod")
		}
		dir = parent
	}
}

func envDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func readCRDs(path string) (map[string]map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	dec := yaml.NewYAMLOrJSONDecoder(f, 1024*1024)
	crds := map[string]map[string]any{}
	for {
		doc := map[string]any{}
		if err := dec.Decode(&doc); err != nil {
			if errors.Is(err, io.EOF) {
				return crds, nil
			}
			return nil, err
		}
		if len(doc) == 0 || stringField(doc, "kind") != "CustomResourceDefinition" {
			continue
		}
		name := stringField(mapField(doc, "metadata"), "name")
		if name == "" {
			return nil, fmt.Errorf("CRD in %s has no metadata.name", path)
		}
		crds[name] = doc
	}
}

func assertContractLabel(t *testing.T, crdName string, doc map[string]any) {
	t.Helper()

	labels := mapField(mapField(doc, "metadata"), "labels")
	got := stringField(labels, contractLabel)
	if got != "v1alpha1" {
		t.Fatalf("%s label %s = %q, want v1alpha1", crdName, contractLabel, got)
	}
}

func assertV1Beta2ServedAlias(t *testing.T, crdName string, doc map[string]any) {
	t.Helper()

	versions, ok := mapField(doc, "spec")["versions"].([]any)
	if !ok {
		t.Fatalf("%s spec.versions is missing or not a list", crdName)
	}

	for _, raw := range versions {
		version, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if stringField(version, "name") != "v1beta2" {
			continue
		}
		if boolField(version, "served") != true {
			t.Fatalf("%s v1beta2 served = false, want true", crdName)
		}
		if boolField(version, "storage") != false {
			t.Fatalf("%s v1beta2 storage = true, want false", crdName)
		}
		return
	}
	t.Fatalf("%s missing v1beta2 served alias", crdName)
}

func mapField(m map[string]any, key string) map[string]any {
	v, _ := m[key].(map[string]any)
	return v
}

func stringField(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func boolField(m map[string]any, key string) bool {
	v, _ := m[key].(bool)
	return v
}
