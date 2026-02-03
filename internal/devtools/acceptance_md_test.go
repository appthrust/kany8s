package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcceptanceMDMentionsKroInfraReflection(t *testing.T) {
	root := findRepoRoot(t)

	path := filepath.Join(root, "acceptance.md")
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}

	acceptance := string(bytes)
	want := "make test-acceptance-kro-infra-reflection"
	if !strings.Contains(acceptance, want) {
		t.Errorf("%s missing %q", filepath.ToSlash(path), want)
	}
}
