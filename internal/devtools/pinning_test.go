package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
)

func findRepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found from %q", dir)
		}
		dir = parent
	}
}

func TestToolingPinned(t *testing.T) {
	root := findRepoRoot(t)

	modPath := filepath.Join(root, "go.mod")
	modBytes, err := os.ReadFile(modPath)
	if err != nil {
		t.Fatalf("read %q: %v", modPath, err)
	}

	f, err := modfile.Parse(modPath, modBytes, nil)
	if err != nil {
		t.Fatalf("parse %q: %v", modPath, err)
	}

	if f.Go == nil {
		t.Fatalf("go.mod missing go directive")
	}
	if got, want := f.Go.Version, "1.25.3"; got != want {
		t.Fatalf("go directive mismatch: got %q, want %q", got, want)
	}

	if f.Toolchain == nil {
		t.Fatalf("go.mod missing toolchain directive")
	}
	if got, want := f.Toolchain.Name, "go1.25.5"; got != want {
		t.Fatalf("toolchain directive mismatch: got %q, want %q", got, want)
	}

	wantTools := []string{
		"github.com/golangci/golangci-lint/v2/cmd/golangci-lint",
		"sigs.k8s.io/controller-runtime/tools/setup-envtest",
		"sigs.k8s.io/controller-tools/cmd/controller-gen",
		"sigs.k8s.io/kustomize/kustomize/v5",
	}
	gotTools := map[string]bool{}
	for _, tool := range f.Tool {
		gotTools[tool.Path] = true
	}
	for _, want := range wantTools {
		if !gotTools[want] {
			t.Errorf("missing tool declaration in go.mod: %q", want)
		}
	}

	wantRequires := map[string]string{
		"github.com/golangci/golangci-lint/v2":               "v2.7.2",
		"sigs.k8s.io/controller-runtime":                     "v0.23.0",
		"sigs.k8s.io/controller-runtime/tools/setup-envtest": "v0.0.0-20260119141314-129853d4ae05",
		"sigs.k8s.io/controller-tools":                       "v0.20.0",
		"sigs.k8s.io/kustomize/kustomize/v5":                 "v5.7.1",
	}
	gotRequires := map[string]string{}
	for _, req := range f.Require {
		gotRequires[req.Mod.Path] = req.Mod.Version
	}
	for path, want := range wantRequires {
		if got, ok := gotRequires[path]; !ok {
			t.Errorf("missing require in go.mod: %q", path)
		} else if got != want {
			t.Errorf("require version mismatch for %q: got %q, want %q", path, got, want)
		}
	}

	toolsGoPath := filepath.Join(root, "hack", "tools.go")
	toolsGoBytes, err := os.ReadFile(toolsGoPath)
	if err != nil {
		t.Fatalf("read %q: %v", toolsGoPath, err)
	}
	toolsGo := string(toolsGoBytes)
	for _, want := range wantTools {
		if !strings.Contains(toolsGo, want) {
			t.Errorf("hack/tools.go missing import: %q", want)
		}
	}

	makefilePath := filepath.Join(root, "Makefile")
	makefileBytes, err := os.ReadFile(makefilePath)
	if err != nil {
		t.Fatalf("read %q: %v", makefilePath, err)
	}
	makefile := string(makefileBytes)
	if !strings.Contains(makefile, "ENVTEST_VERSION ?= $(call gomodver,sigs.k8s.io/controller-runtime/tools/setup-envtest)") {
		t.Errorf("Makefile ENVTEST_VERSION should be pinned using go.mod version")
	}
}
