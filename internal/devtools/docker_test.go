package devtools_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"golang.org/x/mod/modfile"
)

func TestDockerfileBuilderUsesPinnedGoVersion(t *testing.T) {
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
	goVersion := f.Go.Version
	if goVersion == "" {
		t.Fatalf("unexpected go version %q (empty)", f.Go.Version)
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

	if got, want := fromLine[1], goVersion; got != want {
		t.Fatalf("Dockerfile Go builder image tag mismatch: got %q, want %q (from go.mod go directive)", got, want)
	}
}
