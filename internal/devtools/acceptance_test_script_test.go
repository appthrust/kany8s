package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKroReflectionAcceptanceTestScriptExists(t *testing.T) {
	root := findRepoRoot(t)

	scriptPath := filepath.Join(root, "hack", "acceptance-test-kro-reflection.sh")
	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read %q: %v", scriptPath, err)
	}

	script := string(scriptBytes)
	wantSubstrings := []string{
		"#!/usr/bin/env bash",
		"kind create cluster",
		"kro-core-install-manifests.yaml",
		"ResourceGraphAccepted",
		"test/acceptance_test/manifests/kro/rgd.yaml",
		"make deploy",
		"kany8scontrolplane",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(script, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(scriptPath), want)
		}
	}
}

func TestMakefileHasKroReflectionAcceptanceTargets(t *testing.T) {
	root := findRepoRoot(t)

	makefilePath := filepath.Join(root, "Makefile")
	makefileBytes, err := os.ReadFile(makefilePath)
	if err != nil {
		t.Fatalf("read %q: %v", makefilePath, err)
	}

	makefile := string(makefileBytes)
	wantSubstrings := []string{
		"test-acceptance-kro-reflection:",
		"bash hack/acceptance-test-kro-reflection.sh",
		"test-acceptance-kro-reflection-keep:",
		"CLEANUP=false bash hack/acceptance-test-kro-reflection.sh",

		// legacy aliases
		"test-acceptance:",
		"test-acceptance-kro-reflection",
		"test-acceptance-keep:",
		"test-acceptance-kro-reflection-keep",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(makefile, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(makefilePath), want)
		}
	}
}

func TestMakefileHasKroInfraReflectionAcceptanceTargets(t *testing.T) {
	root := findRepoRoot(t)

	makefilePath := filepath.Join(root, "Makefile")
	makefileBytes, err := os.ReadFile(makefilePath)
	if err != nil {
		t.Fatalf("read %q: %v", makefilePath, err)
	}

	makefile := string(makefileBytes)
	wantSubstrings := []string{
		"test-acceptance-kro-infra-reflection:",
		"bash hack/acceptance-test-kro-infra-reflection.sh",
		"test-acceptance-kro-infra-reflection-keep:",
		"CLEANUP=false bash hack/acceptance-test-kro-infra-reflection.sh",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(makefile, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(makefilePath), want)
		}
	}
}

func TestMakefileHasKroInfraClusterIdentityAcceptanceTargets(t *testing.T) {
	root := findRepoRoot(t)

	makefilePath := filepath.Join(root, "Makefile")
	makefileBytes, err := os.ReadFile(makefilePath)
	if err != nil {
		t.Fatalf("read %q: %v", makefilePath, err)
	}

	makefile := string(makefileBytes)
	wantSubstrings := []string{
		"test-acceptance-kro-infra-cluster-identity:",
		"bash hack/acceptance-test-kro-infra-cluster-identity.sh",
		"test-acceptance-kro-infra-cluster-identity-keep:",
		"CLEANUP=false bash hack/acceptance-test-kro-infra-cluster-identity.sh",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(makefile, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(makefilePath), want)
		}
	}
}

func TestLegacyKroAcceptanceScriptDelegates(t *testing.T) {
	root := findRepoRoot(t)

	scriptPath := filepath.Join(root, "hack", "acceptance-test.sh")
	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read %q: %v", scriptPath, err)
	}

	script := string(scriptBytes)
	wantSubstrings := []string{
		"#!/usr/bin/env bash",
		"acceptance-test-kro-reflection.sh",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(script, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(scriptPath), want)
		}
	}
}

func TestCapdKubeadmAcceptanceTestScriptExists(t *testing.T) {
	root := findRepoRoot(t)

	scriptPath := filepath.Join(root, "hack", "acceptance-test-capd-kubeadm.sh")
	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read %q: %v", scriptPath, err)
	}

	script := string(scriptBytes)
	wantSubstrings := []string{
		"#!/usr/bin/env bash",
		"kind create cluster",
		"clusterctl init",
		"--infrastructure docker",
		"--bootstrap kubeadm",
		"--control-plane kany8s",
		"test/acceptance_test/manifests/self-managed-docker/cluster.yaml.tpl",
		"RemoteConnectionProbe",
		"Available",
		"clusterctl get kubeconfig",
		"kubectl --kubeconfig",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(script, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(scriptPath), want)
		}
	}
}

func TestCapdKubeadmAcceptanceTestScriptWaitsForCAPDWebhook(t *testing.T) {
	root := findRepoRoot(t)

	scriptPath := filepath.Join(root, "hack", "acceptance-test-capd-kubeadm.sh")
	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read %q: %v", scriptPath, err)
	}

	script := string(scriptBytes)
	// CAPD uses admission webhooks; applying Docker* resources too early results in
	// connection refused errors. The acceptance script should wait for the webhook Service endpoints.
	wantSubstrings := []string{
		"capd-webhook-service",
		"endpoints",
		"rollout status",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(script, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(scriptPath), want)
		}
	}
}

func TestMakefileHasCapdKubeadmAcceptanceTargets(t *testing.T) {
	root := findRepoRoot(t)

	makefilePath := filepath.Join(root, "Makefile")
	makefileBytes, err := os.ReadFile(makefilePath)
	if err != nil {
		t.Fatalf("read %q: %v", makefilePath, err)
	}

	makefile := string(makefileBytes)
	wantSubstrings := []string{
		"test-acceptance-capd-kubeadm:",
		"bash hack/acceptance-test-capd-kubeadm.sh",
		"test-acceptance-capd-kubeadm-keep:",
		"CLEANUP=false bash hack/acceptance-test-capd-kubeadm.sh",

		// legacy aliases
		"test-acceptance-self-managed:",
		"test-acceptance-capd-kubeadm",
		"test-acceptance-self-managed-keep:",
		"test-acceptance-capd-kubeadm-keep",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(makefile, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(makefilePath), want)
		}
	}
}

func TestLegacySelfManagedAcceptanceScriptDelegates(t *testing.T) {
	root := findRepoRoot(t)

	scriptPath := filepath.Join(root, "hack", "acceptance-test-self-managed.sh")
	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read %q: %v", scriptPath, err)
	}

	script := string(scriptBytes)
	wantSubstrings := []string{
		"#!/usr/bin/env bash",
		"acceptance-test-capd-kubeadm.sh",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(script, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(scriptPath), want)
		}
	}
}

func TestKroReflectionMultiRGDAcceptanceTestScriptExists(t *testing.T) {
	root := findRepoRoot(t)

	scriptPath := filepath.Join(root, "hack", "acceptance-test-kro-reflection-multi-rgd.sh")
	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read %q: %v", scriptPath, err)
	}

	script := string(scriptBytes)
	wantSubstrings := []string{
		"#!/usr/bin/env bash",
		"kind create cluster",
		"kro-core-install-manifests.yaml",
		"ResourceGraphAccepted",
		"demo-control-plane.kro.run",
		"demo-control-plane-alt.kro.run",
		"kany8scontrolplane",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(script, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(scriptPath), want)
		}
	}
}

func TestMakefileHasKroReflectionMultiRGDAcceptanceTargets(t *testing.T) {
	root := findRepoRoot(t)

	makefilePath := filepath.Join(root, "Makefile")
	makefileBytes, err := os.ReadFile(makefilePath)
	if err != nil {
		t.Fatalf("read %q: %v", makefilePath, err)
	}

	makefile := string(makefileBytes)
	wantSubstrings := []string{
		"test-acceptance-kro-reflection-multi-rgd:",
		"bash hack/acceptance-test-kro-reflection-multi-rgd.sh",
		"test-acceptance-kro-reflection-multi-rgd-keep:",
		"CLEANUP=false bash hack/acceptance-test-kro-reflection-multi-rgd.sh",

		// legacy aliases
		"test-acceptance-multi-rgd:",
		"test-acceptance-kro-reflection-multi-rgd",
		"test-acceptance-multi-rgd-keep:",
		"test-acceptance-kro-reflection-multi-rgd-keep",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(makefile, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(makefilePath), want)
		}
	}
}

func TestLegacyKroMultiRGDAcceptanceScriptDelegates(t *testing.T) {
	root := findRepoRoot(t)

	scriptPath := filepath.Join(root, "hack", "acceptance-test-kro-multi-rgd.sh")
	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read %q: %v", scriptPath, err)
	}

	script := string(scriptBytes)
	wantSubstrings := []string{
		"#!/usr/bin/env bash",
		"acceptance-test-kro-reflection-multi-rgd.sh",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(script, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(scriptPath), want)
		}
	}
}
