package devtools_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
)

func TestDockerfileBuilderUsesPinnedGoToolchain(t *testing.T) {
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
	if f.Toolchain == nil {
		t.Fatalf("go.mod missing toolchain directive")
	}
	if !strings.HasPrefix(f.Toolchain.Name, "go") {
		t.Fatalf("unexpected toolchain name %q (expected prefix 'go')", f.Toolchain.Name)
	}
	toolchainVersion := strings.TrimPrefix(f.Toolchain.Name, "go")
	if toolchainVersion == "" {
		t.Fatalf("unexpected toolchain name %q (empty version)", f.Toolchain.Name)
	}

	dockerfilePath := filepath.Join(root, "Dockerfile")
	dockerfileBytes, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("read %q: %v", dockerfilePath, err)
	}
	dockerfile := string(dockerfileBytes)

	fromLine := regexp.MustCompile(`(?m)^FROM\s+golang:([^\s]+)\s+AS\s+builder\s*$`).FindStringSubmatch(dockerfile)
	if fromLine == nil {
		t.Fatalf("Dockerfile missing builder stage matching ^FROM golang:<tag> AS builder$")
	}

	if got, want := fromLine[1], toolchainVersion; got != want {
		t.Fatalf("Dockerfile Go builder image tag mismatch: got %q, want %q (from go.mod toolchain)", got, want)
	}
}
