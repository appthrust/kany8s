package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunbooksCapdKubeadmExists(t *testing.T) {
	root := findRepoRoot(t)

	runbookPath := filepath.Join(root, "docs", "runbooks", "capd-kubeadm.md")
	runbookBytes, err := os.ReadFile(runbookPath)
	if err != nil {
		t.Fatalf("read %q: %v", runbookPath, err)
	}

	runbook := string(runbookBytes)
	wantSubstrings := []string{
		"# CAPD + kubeadm",
		"kind create cluster",
		"clusterctl init",
		"--infrastructure docker",
		"--bootstrap kubeadm",
		"--control-plane kany8s",
		"make docker-build",
		"kind load docker-image",
		"make build-installer",
		"dist/install.yaml",
		"examples/self-managed-docker/cluster.yaml",
		"Cluster Available=True",
		"clusterctl get kubeconfig",
		"kubectl get nodes",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(runbook, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(runbookPath), want)
		}
	}
}
