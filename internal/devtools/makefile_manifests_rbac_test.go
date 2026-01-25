package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMakeManifestsGeneratesRBAC(t *testing.T) {
	root := findRepoRoot(t)

	makefilePath := filepath.Join(root, "Makefile")
	makefileBytes, err := os.ReadFile(makefilePath)
	if err != nil {
		t.Fatalf("read %q: %v", makefilePath, err)
	}

	makefile := string(makefileBytes)
	if !strings.Contains(makefile, "output:rbac:artifacts:config=config/rbac") {
		t.Errorf("%s missing %q", filepath.ToSlash(makefilePath), "output:rbac:artifacts:config=config/rbac")
	}
}
