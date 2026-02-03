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
		"ARTIFACTS_DIR=\"${ARTIFACTS_DIR:-/tmp/kany8s-acceptance-kro-infra-${timestamp}}\"",
		"KUBECONFIG_FILE=\"${KUBECONFIG_FILE:-${ARTIFACTS_DIR}/kubeconfig}\"",
		"RGD_NAME=\"demo-infra.kro.run\"",
		"RGD_INSTANCE_CRD=\"demoinfrastructures.kro.run\"",
		"repo_root=\"$(cd \"$(dirname \"${BASH_SOURCE[0]}\")/..\" && pwd)\"",
		"cd \"${repo_root}\"",
		"KRO_RBAC_WORKAROUND_MANIFEST=\"${KRO_RBAC_WORKAROUND_MANIFEST:-test/acceptance_test/manifests/kro/rbac-unrestricted.yaml}\"",
		"KRO_CORE_INSTALL_MANIFEST=\"${KRO_CORE_INSTALL_MANIFEST:-test/acceptance_test/vendor/kro/v${KRO_VERSION}/kro-core-install-manifests.yaml}\"",
		"mkdir -p \"$(dirname \"${KRO_CORE_INSTALL_MANIFEST}\")\"",
		"curl -fsSL -o \"${KRO_CORE_INSTALL_MANIFEST}\"",
		"https://github.com/kubernetes-sigs/kro/releases/download/v${KRO_VERSION}/kro-core-install-manifests.yaml",
		"k apply -f \"${KRO_CORE_INSTALL_MANIFEST}\"",
		"k -n kro-system rollout status deploy/kro --timeout=180s",
		"KRO_RGD_MANIFEST=\"${KRO_RGD_MANIFEST:-test/acceptance_test/manifests/kro/infra/rgd.yaml}\"",
		"KANY8S_CLUSTER_TEMPLATE=\"${KANY8S_CLUSTER_TEMPLATE:-test/acceptance_test/manifests/kro/kany8scluster.yaml.tpl}\"",
		"mkdir -p \"${ARTIFACTS_DIR}\"",
		"export KUBECONFIG=\"${KUBECONFIG_FILE}\"",
		"log_file=\"${ARTIFACTS_DIR}/acceptance-infra.log\"",
		"exec > >(tee -a \"${log_file}\") 2>&1",
		"kustomization_path=\"${repo_root}/config/manager/kustomization.yaml\"",
		"need_cmd()",
		"need_cmd docker",
		"need_cmd kind",
		"need_cmd kubectl",
		"need_cmd make",
		"need_cmd go",
		"need_cmd curl",
		"kind create cluster --name \"${KIND_CLUSTER_NAME}\" --wait 60s --kubeconfig \"${KUBECONFIG_FILE}\"",
		"k get namespace kro-system >/dev/null 2>&1 || k create namespace kro-system",
		"k() {",
		"kubectl --context \"${KUBECTL_CONTEXT}\"",
		"backup_kustomization()",
		"restore_kustomization()",
		"cleanup() {\n\trestore_kustomization",
		"if [[ \"${CLEANUP}\" == \"true\" ]]; then",
		"kind delete cluster --name \"${KIND_CLUSTER_NAME}\" --kubeconfig \"${KUBECONFIG_FILE}\"",
		"CLEANUP=false; keeping kind cluster",
		"collect_diagnostics() {",
		"kind get clusters",
		"kubeconfig-contexts.txt",
		"kubeconfig-minify.yaml",
		"nodes.txt",
		"events.txt",
		"kro-system.txt",
		"kro-logs.txt",
		"rgd-instance-crd.yaml",
		"kany8s-controller-logs.txt",
		"kany8scluster.yaml",
		"rgd-instance.yaml",
		"on_exit() {",
		"rc=$?",
		"collect_diagnostics || true",
		"cleanup || true",
		"trap on_exit EXIT",
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
