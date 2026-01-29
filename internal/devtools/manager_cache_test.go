package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMainDisablesCacheForCoreSecrets(t *testing.T) {
	root := findRepoRoot(t)

	mainPath := filepath.Join(root, "cmd", "main.go")
	mainBytes, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("read %q: %v", mainPath, err)
	}

	mainGo := string(mainBytes)
	// The manager's default client reads via the controller-runtime cache, which requires
	// list/watch RBAC for the requested object type. We intentionally do not grant list/watch
	// for core Secrets, so we must explicitly disable caching for Secrets.
	wantSubstrings := []string{
		"DisableFor:",
		"&corev1.Secret{}",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(mainGo, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(mainPath), want)
		}
	}
}
