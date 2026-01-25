package devtools_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"sigs.k8s.io/yaml"
)

type kustomization struct {
	Namespace  string   `yaml:"namespace"`
	NamePrefix string   `yaml:"namePrefix"`
	Resources  []string `yaml:"resources"`
	Patches    []struct {
		Path string `yaml:"path"`
	} `yaml:"patches"`
}

func TestConfigDefaultIsReadyForMakeDeploy(t *testing.T) {
	root := findRepoRoot(t)

	kustomizationPath := filepath.Join(root, "config", "default", "kustomization.yaml")
	kustomizationBytes, err := os.ReadFile(kustomizationPath)
	if err != nil {
		t.Fatalf("read %q: %v", kustomizationPath, err)
	}

	var k kustomization
	if err := yaml.Unmarshal(kustomizationBytes, &k); err != nil {
		t.Fatalf("parse %q: %v", kustomizationPath, err)
	}

	if got, want := k.Namespace, "kany8s-system"; got != want {
		t.Errorf("%s namespace mismatch: got %q, want %q", filepath.ToSlash(kustomizationPath), got, want)
	}
	if got, want := k.NamePrefix, "kany8s-"; got != want {
		t.Errorf("%s namePrefix mismatch: got %q, want %q", filepath.ToSlash(kustomizationPath), got, want)
	}

	for _, want := range []string{"../crd", "../rbac", "../manager", "metrics_service.yaml"} {
		if !slices.Contains(k.Resources, want) {
			t.Errorf("%s missing resources entry %q", filepath.ToSlash(kustomizationPath), want)
		}
	}

	patchPaths := []string{}
	for _, patch := range k.Patches {
		patchPaths = append(patchPaths, patch.Path)
	}
	for _, want := range []string{"manager_metrics_patch.yaml"} {
		if !slices.Contains(patchPaths, want) {
			t.Errorf("%s missing patches entry %q", filepath.ToSlash(kustomizationPath), want)
		}
	}

	for _, rel := range []string{"manager_metrics_patch.yaml", "metrics_service.yaml"} {
		path := filepath.Join(root, "config", "default", rel)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("missing file %q: %v", filepath.ToSlash(path), err)
		}
	}
}
