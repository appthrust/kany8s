package devtools_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKany8sControlPlaneControllerScaffoldExists(t *testing.T) {
	root := findRepoRoot(t)

	controllerPath := filepath.Join(root, "internal", "controller", "kany8scontrolplane_controller.go")
	controllerBytes, err := os.ReadFile(controllerPath)
	if err != nil {
		t.Fatalf("read %q: %v", controllerPath, err)
	}

	controllerGo := string(controllerBytes)
	wantControllerSubstrings := []string{
		"type Kany8sControlPlaneReconciler struct",
		"func (r *Kany8sControlPlaneReconciler) Reconcile",
		"SetupWithManager",
	}
	for _, want := range wantControllerSubstrings {
		if !strings.Contains(controllerGo, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(controllerPath), want)
		}
	}

	mainPath := filepath.Join(root, "cmd", "main.go")
	mainBytes, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("read %q: %v", mainPath, err)
	}

	mainGo := string(mainBytes)
	wantMainSubstrings := []string{
		"Kany8sControlPlaneReconciler",
		"SetupWithManager(mgr)",
	}
	for _, want := range wantMainSubstrings {
		if !strings.Contains(mainGo, want) {
			t.Errorf("%s missing %q", filepath.ToSlash(mainPath), want)
		}
	}
}
