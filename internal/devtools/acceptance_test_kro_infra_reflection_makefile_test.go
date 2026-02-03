package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMakefileHasKroInfraReflectionAcceptanceTargetsMarkedPhony(t *testing.T) {
	root := findRepoRoot(t)

	makefilePath := filepath.Join(root, "Makefile")
	makefileBytes, err := os.ReadFile(makefilePath)
	if err != nil {
		t.Fatalf("read %q: %v", makefilePath, err)
	}

	makefile := string(makefileBytes)
	wantSubstrings := []string{
		".PHONY: test-acceptance-kro-infra-reflection",
		".PHONY: test-acceptance-kro-infra-reflection-keep",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(makefile, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(makefilePath), want)
		}
	}
}
