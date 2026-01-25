package kubeconfig

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestSecretName(t *testing.T) {
	t.Parallel()

	got, err := SecretName("demo")
	if err != nil {
		t.Fatalf("SecretName returned error: %v", err)
	}
	if want := "demo-kubeconfig"; got != want {
		t.Fatalf("SecretName returned %q, want %q", got, want)
	}
}

func TestSecretName_Errors(t *testing.T) {
	t.Parallel()

	_, err := SecretName("   ")
	if err == nil {
		t.Fatalf("SecretName unexpectedly succeeded")
	}
}

func TestNewSecret(t *testing.T) {
	t.Parallel()

	kubeconfig := []byte("kubeconfig-bytes")

	got, err := NewSecret("demo", "default", kubeconfig)
	if err != nil {
		t.Fatalf("NewSecret returned error: %v", err)
	}

	if got.Name != "demo-kubeconfig" {
		t.Fatalf("secret name = %q, want %q", got.Name, "demo-kubeconfig")
	}
	if got.Namespace != "default" {
		t.Fatalf("secret namespace = %q, want %q", got.Namespace, "default")
	}
	if got.Type != SecretType {
		t.Fatalf("secret type = %q, want %q", got.Type, SecretType)
	}
	if got.Labels[ClusterNameLabelKey] != "demo" {
		t.Fatalf("secret label %q = %q, want %q", ClusterNameLabelKey, got.Labels[ClusterNameLabelKey], "demo")
	}
	if string(got.Data[DataKey]) != string(kubeconfig) {
		t.Fatalf("secret data[%q] = %q, want %q", DataKey, string(got.Data[DataKey]), string(kubeconfig))
	}

	// Ensure we use the CAPI-expected Secret type (not corev1.SecretTypeOpaque).
	if got.Type == corev1.SecretTypeOpaque {
		t.Fatalf("secret type unexpectedly set to Opaque")
	}
}

func TestNewSecret_Errors(t *testing.T) {
	t.Parallel()

	_, err := NewSecret("", "default", []byte("x"))
	if err == nil {
		t.Fatalf("NewSecret unexpectedly succeeded with empty cluster name")
	}

	_, err = NewSecret("demo", "", []byte("x"))
	if err == nil {
		t.Fatalf("NewSecret unexpectedly succeeded with empty namespace")
	}
}
