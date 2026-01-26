package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcceptanceTestScriptExists(t *testing.T) {
	root := findRepoRoot(t)

	scriptPath := filepath.Join(root, "hack", "acceptance-test.sh")
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
		"examples/kro/ready-endpoint/rgd.yaml",
		"make deploy",
		"kany8scontrolplane",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(script, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(scriptPath), want)
		}
	}
}

func TestMakefileHasAcceptanceTarget(t *testing.T) {
	root := findRepoRoot(t)

	makefilePath := filepath.Join(root, "Makefile")
	makefileBytes, err := os.ReadFile(makefilePath)
	if err != nil {
		t.Fatalf("read %q: %v", makefilePath, err)
	}

	makefile := string(makefileBytes)
	wantSubstrings := []string{
		"test-acceptance:",
		"hack/acceptance-test.sh",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(makefile, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(makefilePath), want)
		}
	}
}
