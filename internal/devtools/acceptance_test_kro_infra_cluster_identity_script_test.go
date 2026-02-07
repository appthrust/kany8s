package devtools_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestKroInfraClusterIdentityAcceptanceHackScriptIsExecutable(t *testing.T) {
	if runtime.GOOS == goosWindows {
		t.Skip("executable bit is not enforced on windows")
	}

	root := findRepoRoot(t)

	scriptPath := filepath.Join(root, "hack", "acceptance-test-kro-infra-cluster-identity.sh")
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("stat %q: %v", scriptPath, err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("%s is not a regular file", filepath.ToSlash(scriptPath))
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("%s is not executable (mode=%#o)", filepath.ToSlash(scriptPath), info.Mode().Perm())
	}
}

func TestKroInfraClusterIdentityAcceptanceWrapperScriptIsExecutable(t *testing.T) {
	if runtime.GOOS == goosWindows {
		t.Skip("executable bit is not enforced on windows")
	}

	root := findRepoRoot(t)

	scriptPath := filepath.Join(root, "test", "acceptance_test", "run-acceptance-kro-infra-cluster-identity.sh")
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("stat %q: %v", scriptPath, err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("%s is not a regular file", filepath.ToSlash(scriptPath))
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("%s is not executable (mode=%#o)", filepath.ToSlash(scriptPath), info.Mode().Perm())
	}
}

func TestKroInfraClusterIdentityAcceptanceHackScriptExists(t *testing.T) {
	root := findRepoRoot(t)

	scriptPath := filepath.Join(root, "hack", "acceptance-test-kro-infra-cluster-identity.sh")
	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read %q: %v", scriptPath, err)
	}

	script := string(scriptBytes)
	wantSubstrings := []string{
		"#!/usr/bin/env bash",
		"set -euo pipefail",
		`timestamp="$(date +%Y%m%d%H%M%S)"`,
		"RGD_NAME=\"demo-infra-ownerref.kro.run\"",
		"RGD_INSTANCE_CRD=\"demoinfrastructureowneds.kro.run\"",
		"rgd-ownerref.yaml",
		"cluster.yaml.tpl",
		"clusterctl",
		"clusterctl-linux-amd64",
		"clusterctl_bin",
		"clusterctl ${CLUSTERCTL_VERSION}",
		"init --infrastructure docker --wait-providers",
		"kind create cluster",
		"ResourceGraphAccepted",
		"make deploy",
		"kany8scluster",
		"spec.clusterUID",
		"ownerReferences",
		"cluster.x-k8s.io/cluster-name",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(script, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(scriptPath), want)
		}
	}
}

func TestKroInfraClusterIdentityAcceptanceHackScriptUsesStrictMode(t *testing.T) {
	root := findRepoRoot(t)

	scriptPath := filepath.Join(root, "hack", "acceptance-test-kro-infra-cluster-identity.sh")
	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read %q: %v", scriptPath, err)
	}

	lines := strings.Split(string(scriptBytes), "\n")
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSuffix(lines[i], "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if trimmed != "set -euo pipefail" {
			t.Fatalf("%s first command line=%q want %q", filepath.ToSlash(scriptPath), trimmed, "set -euo pipefail")
		}
		return
	}

	t.Fatalf("%s missing %q", filepath.ToSlash(scriptPath), "set -euo pipefail")
}

func TestKroInfraClusterIdentityAcceptanceHackScriptHasValidBashSyntax(t *testing.T) {
	if runtime.GOOS == goosWindows {
		t.Skip("bash -n is not supported on windows")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found")
	}

	root := findRepoRoot(t)

	scriptPath := filepath.Join(root, "hack", "acceptance-test-kro-infra-cluster-identity.sh")
	cmd := exec.Command("bash", "-n", scriptPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bash -n %s: %v\n%s", filepath.ToSlash(scriptPath), err, string(out))
	}
}

func TestKroInfraClusterIdentityAcceptanceWrapperScriptExists(t *testing.T) {
	root := findRepoRoot(t)

	scriptPath := filepath.Join(root, "test", "acceptance_test", "run-acceptance-kro-infra-cluster-identity.sh")
	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read %q: %v", scriptPath, err)
	}

	script := string(scriptBytes)
	wantSubstrings := []string{
		"#!/usr/bin/env bash",
		"set -euo pipefail",
		`timestamp="${TIMESTAMP:-$(date +%Y%m%d%H%M%S)}"`,
		`repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"`,
		"kany8s-acc-infra-identity-",
		"kany8s-acceptance-kro-infra-cluster-identity-",
		"kind delete cluster",
		"acceptance-test-kro-infra-cluster-identity.sh",
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

func TestKroInfraClusterIdentityAcceptanceWrapperScriptHasValidBashSyntax(t *testing.T) {
	if runtime.GOOS == goosWindows {
		t.Skip("bash -n is not supported on windows")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found")
	}

	root := findRepoRoot(t)

	scriptPath := filepath.Join(root, "test", "acceptance_test", "run-acceptance-kro-infra-cluster-identity.sh")
	cmd := exec.Command("bash", "-n", scriptPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bash -n %s: %v\n%s", filepath.ToSlash(scriptPath), err, string(out))
	}
}
