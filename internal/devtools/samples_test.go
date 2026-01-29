package devtools_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

type samplesKustomization struct {
	Resources []string `yaml:"resources"`
}

func TestConfigSamplesKustomizationIncludesAllSampleResources(t *testing.T) {
	root := findRepoRoot(t)

	kustomizationPath := filepath.Join(root, "config", "samples", "kustomization.yaml")
	kustomizationBytes, err := os.ReadFile(kustomizationPath)
	if err != nil {
		t.Fatalf("read %q: %v", kustomizationPath, err)
	}

	var k samplesKustomization
	if err := yaml.Unmarshal(kustomizationBytes, &k); err != nil {
		t.Fatalf("parse %q: %v", kustomizationPath, err)
	}

	for _, want := range []string{
		"controlplane_v1alpha1_kany8scontrolplane.yaml",
		"controlplane_v1alpha1_kany8skubeadmcontrolplane.yaml",
		"controlplane_v1alpha1_kany8scontrolplanetemplate.yaml",
		"infrastructure_v1alpha1_kany8scluster.yaml",
		"infrastructure_v1alpha1_kany8sclustertemplate.yaml",
	} {
		if !slices.Contains(k.Resources, want) {
			t.Errorf("%s missing resources entry %q", filepath.ToSlash(kustomizationPath), want)
		}
	}
}

func TestConfigSamplesAreFilledInAndMatchCRDContracts(t *testing.T) {
	root := findRepoRoot(t)

	type sampleSpec struct {
		relPath         string
		wantAPIVersion  string
		wantKind        string
		requiredStrings [][]string
		requiredMaps    [][]string
	}

	samples := []sampleSpec{
		{
			relPath:        filepath.Join("config", "samples", "controlplane_v1alpha1_kany8scontrolplane.yaml"),
			wantAPIVersion: "controlplane.cluster.x-k8s.io/v1alpha1",
			wantKind:       "Kany8sControlPlane",
			requiredStrings: [][]string{
				{"metadata", "name"},
				{"spec", "version"},
				{"spec", "resourceGraphDefinitionRef", "name"},
			},
		},
		{
			relPath:        filepath.Join("config", "samples", "controlplane_v1alpha1_kany8skubeadmcontrolplane.yaml"),
			wantAPIVersion: "controlplane.cluster.x-k8s.io/v1alpha1",
			wantKind:       "Kany8sKubeadmControlPlane",
			requiredStrings: [][]string{
				{"metadata", "name"},
				{"spec", "version"},
				{"spec", "machineTemplate", "infrastructureRef", "apiVersion"},
				{"spec", "machineTemplate", "infrastructureRef", "kind"},
				{"spec", "machineTemplate", "infrastructureRef", "name"},
			},
			requiredMaps: [][]string{
				{"spec"},
				{"spec", "machineTemplate"},
				{"spec", "machineTemplate", "infrastructureRef"},
			},
		},
		{
			relPath:        filepath.Join("config", "samples", "controlplane_v1alpha1_kany8scontrolplanetemplate.yaml"),
			wantAPIVersion: "controlplane.cluster.x-k8s.io/v1alpha1",
			wantKind:       "Kany8sControlPlaneTemplate",
			requiredStrings: [][]string{
				{"metadata", "name"},
				{"spec", "template", "spec", "resourceGraphDefinitionRef", "name"},
			},
		},
		{
			relPath:        filepath.Join("config", "samples", "infrastructure_v1alpha1_kany8scluster.yaml"),
			wantAPIVersion: "infrastructure.cluster.x-k8s.io/v1alpha1",
			wantKind:       "Kany8sCluster",
			requiredStrings: [][]string{
				{"metadata", "name"},
			},
			requiredMaps: [][]string{
				{"spec"},
			},
		},
		{
			relPath:        filepath.Join("config", "samples", "infrastructure_v1alpha1_kany8sclustertemplate.yaml"),
			wantAPIVersion: "infrastructure.cluster.x-k8s.io/v1alpha1",
			wantKind:       "Kany8sClusterTemplate",
			requiredStrings: [][]string{
				{"metadata", "name"},
			},
			requiredMaps: [][]string{
				{"spec", "template", "spec"},
			},
		},
	}

	for _, sample := range samples {
		samplePath := filepath.Join(root, sample.relPath)
		sampleBytes, err := os.ReadFile(samplePath)
		if err != nil {
			t.Fatalf("read %q: %v", samplePath, err)
		}

		sampleText := string(sampleBytes)
		if strings.Contains(sampleText, "TODO(user): Add fields here") {
			t.Errorf("%s still contains kubebuilder TODO placeholder", filepath.ToSlash(samplePath))
		}

		var obj map[string]any
		if err := yaml.Unmarshal(sampleBytes, &obj); err != nil {
			t.Fatalf("parse %q: %v", samplePath, err)
		}

		if got := mustStringAtPath(t, obj, samplePath, []string{"apiVersion"}); got != sample.wantAPIVersion {
			t.Errorf("%s apiVersion mismatch: got %q, want %q", filepath.ToSlash(samplePath), got, sample.wantAPIVersion)
		}
		if got := mustStringAtPath(t, obj, samplePath, []string{"kind"}); got != sample.wantKind {
			t.Errorf("%s kind mismatch: got %q, want %q", filepath.ToSlash(samplePath), got, sample.wantKind)
		}

		for _, p := range sample.requiredStrings {
			_ = mustStringAtPath(t, obj, samplePath, p)
		}
		for _, p := range sample.requiredMaps {
			_ = mustMapAtPath(t, obj, samplePath, p)
		}
	}
}

func mustStringAtPath(t *testing.T, obj map[string]any, path string, parts []string) string {
	t.Helper()

	got, ok := lookup(obj, parts)
	if !ok {
		t.Fatalf("%s missing required field %q", filepath.ToSlash(path), strings.Join(parts, "."))
	}
	str, ok := got.(string)
	if !ok {
		t.Fatalf("%s field %q is not a string (got %T)", filepath.ToSlash(path), strings.Join(parts, "."), got)
	}
	if strings.TrimSpace(str) == "" {
		t.Fatalf("%s field %q must be non-empty", filepath.ToSlash(path), strings.Join(parts, "."))
	}
	return str
}

func mustMapAtPath(t *testing.T, obj map[string]any, path string, parts []string) map[string]any {
	t.Helper()

	got, ok := lookup(obj, parts)
	if !ok {
		t.Fatalf("%s missing required field %q", filepath.ToSlash(path), strings.Join(parts, "."))
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("%s field %q is not an object (got %T)", filepath.ToSlash(path), strings.Join(parts, "."), got)
	}
	return m
}

func lookup(obj map[string]any, parts []string) (any, bool) {
	cur := any(obj)
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok := m[p]
		if !ok {
			return nil, false
		}
		cur = v
	}
	return cur, true
}
