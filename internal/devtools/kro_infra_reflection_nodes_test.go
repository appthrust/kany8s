package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKroInfraReflectionAcceptanceScriptListsNodesAfterKindCreate(t *testing.T) {
	root := findRepoRoot(t)

	scriptPath := filepath.Join(root, "hack", "acceptance-test-kro-infra-reflection.sh")
	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read %q: %v", scriptPath, err)
	}

	script := string(scriptBytes)
	kindCreateIdx := strings.Index(script, "kind create cluster")
	if kindCreateIdx == -1 {
		t.Fatalf("%s missing %q", filepath.ToSlash(scriptPath), "kind create cluster")
	}

	wantLine := "k get nodes -o wide\n"
	nodesIdx := strings.Index(script, wantLine)
	if nodesIdx == -1 {
		t.Errorf("%s missing %q", filepath.ToSlash(scriptPath), strings.TrimSpace(wantLine))
		return
	}
	if nodesIdx < kindCreateIdx {
		t.Errorf("%s contains %q before %q", filepath.ToSlash(scriptPath), strings.TrimSpace(wantLine), "kind create cluster")
	}
}
