package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKany8sClusterControllerIncludesKroRBACMarkers(t *testing.T) {
	root := findRepoRoot(t)

	controllerPath := filepath.Join(root, "internal", "controller", "infrastructure", "kany8scluster_controller.go")
	controllerBytes, err := os.ReadFile(controllerPath)
	if err != nil {
		t.Fatalf("read %q: %v", controllerPath, err)
	}

	controllerGo := string(controllerBytes)
	wantSubstrings := []string{
		"+kubebuilder:rbac:groups=kro.run,resources=resourcegraphdefinitions,verbs=get;list;watch",
		"+kubebuilder:rbac:groups=kro.run,resources=*,verbs=get;list;watch;create;update;patch",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(controllerGo, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(controllerPath), want)
		}
	}
}
