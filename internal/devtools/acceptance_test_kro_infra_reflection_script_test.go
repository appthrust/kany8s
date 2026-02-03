package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKroInfraReflectionAcceptanceTestScriptExists(t *testing.T) {
	root := findRepoRoot(t)

	scriptPath := filepath.Join(root, "hack", "acceptance-test-kro-infra-reflection.sh")
	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read %q: %v", scriptPath, err)
	}

	script := string(scriptBytes)
	wantSubstrings := []string{
		"#!/usr/bin/env bash",
		"set -euo pipefail",
		"KIND_CLUSTER_NAME=\"${KIND_CLUSTER_NAME:-kany8s-acceptance-infra-${timestamp}}\"",
		"KUBECTL_CONTEXT=\"${KUBECTL_CONTEXT:-kind-${KIND_CLUSTER_NAME}}\"",
		"NAMESPACE=\"${NAMESPACE:-default}\"",
		"CLUSTER_NAME=\"${CLUSTER_NAME:-demo-cluster}\"",
		"KRO_VERSION=\"${KRO_VERSION:-0.7.1}\"",
		"IMG=\"${IMG:-example.com/kany8s:acceptance-kro-infra}\"",
		"CLEANUP=\"${CLEANUP:-true}\"",
		"RGD_NAME=\"demo-infra.kro.run\"",
		"RGD_INSTANCE_CRD=\"demoinfrastructures.kro.run\"",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(script, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(scriptPath), want)
		}
	}
}

func TestKroInfraReflectionAcceptanceWrapperScriptExists(t *testing.T) {
	root := findRepoRoot(t)

	scriptPath := filepath.Join(root, "test", "acceptance_test", "run-acceptance-kro-infra-reflection.sh")
	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read %q: %v", scriptPath, err)
	}

	script := string(scriptBytes)
	wantSubstrings := []string{
		"#!/usr/bin/env bash",
		"set -euo pipefail",
		"kany8s-acc-infra-",
		"kany8s-acceptance-kro-infra-reflection-",
		"kind delete cluster",
		"acceptance-test-kro-infra-reflection.sh",
		"KIND_CLUSTER_NAME=\"${KIND_CLUSTER_NAME}\"",
		"ARTIFACTS_DIR=\"${ARTIFACTS_DIR}\"",
		"CLEANUP=\"${CLEANUP}\"",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(script, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(scriptPath), want)
		}
	}
}
