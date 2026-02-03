package devtools_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const goosWindows = "windows"

func TestAcceptanceRunAllScriptIsExecutable(t *testing.T) {
	if runtime.GOOS == goosWindows {
		t.Skip("executable bit is not enforced on windows")
	}

	root := findRepoRoot(t)

	scriptPath := filepath.Join(root, "test", "acceptance_test", "run-all.sh")
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

func TestAcceptanceRunAllScriptRunsKroInfraReflection(t *testing.T) {
	root := findRepoRoot(t)

	scriptPath := filepath.Join(root, "test", "acceptance_test", "run-all.sh")
	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read %q: %v", scriptPath, err)
	}

	script := string(scriptBytes)
	wantSubstrings := []string{
		"#!/usr/bin/env bash",
		"set -euo pipefail",
		"==> Acceptance run-all",
		"==> Running: kro acceptance (kro infra reflection)",
		"ARTIFACTS_DIR=\"${base_artifacts_dir}/acceptance-kro-infra-reflection\"",
		"KIND_CLUSTER_NAME=\"kany8s-acc-infra-${timestamp}\"",
		"run-acceptance-kro-infra-reflection.sh",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(script, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(scriptPath), want)
		}
	}
}

func TestAcceptanceRunAllScriptHasValidBashSyntax(t *testing.T) {
	if runtime.GOOS == goosWindows {
		t.Skip("bash -n is not supported on windows")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found")
	}

	root := findRepoRoot(t)

	scriptPath := filepath.Join(root, "test", "acceptance_test", "run-all.sh")
	cmd := exec.Command("bash", "-n", scriptPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bash -n %s: %v\n%s", filepath.ToSlash(scriptPath), err, string(out))
	}
}
