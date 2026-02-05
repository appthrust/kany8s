package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRgdGuidelinesDocExists(t *testing.T) {
	root := findRepoRoot(t)

	docPath := filepath.Join(root, "docs", "reference", "rgd-guidelines.md")
	docBytes, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read %q: %v", docPath, err)
	}

	doc := string(docBytes)
	wantSubstrings := []string{
		"# RGD Guidelines",
		"static analysis",
		"ResourceGraphAccepted",
		"spec.schema.status",
		"schema.*",
		"readyWhen",
		"CEL",
		"string template",
		"Infra outputs",
		"control plane spec",
		"orValue",
		".?",
		"NetworkPolicy",
		"docs/reference/kro-v0.7.1-kind-notes.md",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(doc, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(docPath), want)
		}
	}
}
