package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadmeDocumentsLocalDevLoop(t *testing.T) {
	root := findRepoRoot(t)

	readmePath := filepath.Join(root, "docs", "README.md")
	readmeBytes, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read %q: %v", readmePath, err)
	}

	readme := string(readmeBytes)
	wantSubstrings := []string{
		"## Development",
		"### Prerequisites",
		"### Quickstart",
		"`make test`",
		"`make lint`",
		"`make generate`",
		"`make manifests`",
		"`make run`",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(readme, want) {
			t.Errorf("docs/README.md missing %q", want)
		}
	}
}

func TestReadmeDocumentsInstallApplyFlow(t *testing.T) {
	root := findRepoRoot(t)

	readmePath := filepath.Join(root, "docs", "README.md")
	readmeBytes, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read %q: %v", readmePath, err)
	}

	readme := string(readmeBytes)
	wantSubstrings := []string{
		"## Demo (kind + kro)",
		"kind create cluster",
		"`make install`",
		"`make run`",
		"examples/kro/ready-endpoint/rgd.yaml",
		"examples/capi/cluster.yaml",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(readme, want) {
			t.Errorf("docs/README.md missing %q", want)
		}
	}
}

func TestReadmeDocumentsKroInfraReflectionAcceptanceRunner(t *testing.T) {
	root := findRepoRoot(t)

	readmePath := filepath.Join(root, "docs", "README.md")
	readmeBytes, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read %q: %v", readmePath, err)
	}

	readme := string(readmeBytes)
	wantSubstrings := []string{
		"test-acceptance-kro-infra-reflection",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(readme, want) {
			t.Errorf("docs/README.md missing %q", want)
		}
	}
}

func TestReadmeDocumentsKroInfraClusterIdentityAcceptanceRunner(t *testing.T) {
	root := findRepoRoot(t)

	readmePath := filepath.Join(root, "docs", "README.md")
	readmeBytes, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read %q: %v", readmePath, err)
	}

	readme := string(readmeBytes)
	wantSubstrings := []string{
		"test-acceptance-kro-infra-cluster-identity",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(readme, want) {
			t.Errorf("docs/README.md missing %q", want)
		}
	}
}
