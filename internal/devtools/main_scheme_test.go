package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMainRegistersClusterAPISchemes(t *testing.T) {
	root := findRepoRoot(t)

	mainPath := filepath.Join(root, "cmd", "main.go")
	mainBytes, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("read %q: %v", mainPath, err)
	}

	mainGo := string(mainBytes)
	wantSubstrings := []string{
		"clusterv1.AddToScheme(scheme)",
		"bootstrapv1.AddToScheme(scheme)",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(mainGo, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(mainPath), want)
		}
	}
}
