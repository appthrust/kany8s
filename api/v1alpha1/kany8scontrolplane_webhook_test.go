package v1alpha1

import (
	"context"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func TestKany8sControlPlaneWebhook_ValidateCreateExactlyOneBackend(t *testing.T) {
	t.Parallel()

	validator := &kany8sControlPlaneValidator{}
	base := Kany8sControlPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"},
		Spec:       Kany8sControlPlaneSpec{Version: "v1.34.0"},
	}

	tests := []struct {
		name       string
		mutate     func(*Kany8sControlPlane)
		wantErr    bool
		wantReason string
	}{
		{
			name: "kro backend is valid",
			mutate: func(cp *Kany8sControlPlane) {
				cp.Spec.ResourceGraphDefinitionRef = &ResourceGraphDefinitionReference{Name: "demo.kro.run"}
			},
		},
		{
			name: "kubeadm backend is valid",
			mutate: func(cp *Kany8sControlPlane) {
				cp.Spec.Kubeadm = &Kany8sControlPlaneKubeadmSpec{
					MachineTemplate: Kany8sKubeadmControlPlaneMachineTemplate{
						InfrastructureRef: clusterv1.ContractVersionedObjectReference{
							APIGroup: "infrastructure.cluster.x-k8s.io",
							Kind:     "DockerMachineTemplate",
							Name:     "demo-control-plane",
						},
					},
				}
			},
		},
		{
			name: "external backend is valid",
			mutate: func(cp *Kany8sControlPlane) {
				cp.Spec.ExternalBackend = &Kany8sControlPlaneExternalBackendSpec{
					APIVersion: "controlplane.example.com/v1alpha1",
					Kind:       "ExampleControlPlane",
				}
			},
		},
		{
			name:    "no backend is invalid",
			mutate:  func(_ *Kany8sControlPlane) {},
			wantErr: true,
		},
		{
			name: "multiple backends are invalid",
			mutate: func(cp *Kany8sControlPlane) {
				cp.Spec.ResourceGraphDefinitionRef = &ResourceGraphDefinitionReference{Name: "demo.kro.run"}
				cp.Spec.Kubeadm = &Kany8sControlPlaneKubeadmSpec{
					MachineTemplate: Kany8sKubeadmControlPlaneMachineTemplate{
						InfrastructureRef: clusterv1.ContractVersionedObjectReference{
							APIGroup: "infrastructure.cluster.x-k8s.io",
							Kind:     "DockerMachineTemplate",
							Name:     "demo-control-plane",
						},
					},
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cp := base.DeepCopy()
			tt.mutate(cp)

			_, err := validator.ValidateCreate(context.Background(), cp)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected validation error")
				}
				if !apierrors.IsInvalid(err) {
					t.Fatalf("expected Invalid error, got %T: %v", err, err)
				}
				if !strings.Contains(err.Error(), "exactly one backend must be selected") {
					t.Fatalf("error = %q, want to contain %q", err.Error(), "exactly one backend must be selected")
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateCreate() error = %v", err)
			}
		})
	}
}

func TestKany8sControlPlaneWebhook_ValidateUpdateBackendTypeImmutable(t *testing.T) {
	t.Parallel()

	validator := &kany8sControlPlaneValidator{}
	oldCP := &Kany8sControlPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"},
		Spec: Kany8sControlPlaneSpec{
			Version:                    "v1.34.0",
			ResourceGraphDefinitionRef: &ResourceGraphDefinitionReference{Name: "demo.kro.run"},
		},
	}

	newCP := oldCP.DeepCopy()
	newCP.Spec.ResourceGraphDefinitionRef = nil
	newCP.Spec.Kubeadm = &Kany8sControlPlaneKubeadmSpec{
		MachineTemplate: Kany8sKubeadmControlPlaneMachineTemplate{
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: "infrastructure.cluster.x-k8s.io",
				Kind:     "DockerMachineTemplate",
				Name:     "demo-control-plane",
			},
		},
	}

	_, err := validator.ValidateUpdate(context.Background(), oldCP, newCP)
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if !apierrors.IsInvalid(err) {
		t.Fatalf("expected Invalid error, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "backend type is immutable") {
		t.Fatalf("error = %q, want to contain %q", err.Error(), "backend type is immutable")
	}
}

func TestKany8sControlPlaneTemplateWebhook_ValidateCreateAndUpdate(t *testing.T) {
	t.Parallel()

	validator := &kany8sControlPlaneTemplateValidator{}

	validTemplate := &Kany8sControlPlaneTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"},
		Spec: Kany8sControlPlaneTemplateSpec{
			Template: Kany8sControlPlaneTemplateResource{
				Spec: Kany8sControlPlaneTemplateResourceSpec{
					ResourceGraphDefinitionRef: &ResourceGraphDefinitionReference{Name: "demo.kro.run"},
				},
			},
		},
	}

	if _, err := validator.ValidateCreate(context.Background(), validTemplate); err != nil {
		t.Fatalf("ValidateCreate() unexpected error: %v", err)
	}

	invalidTemplate := validTemplate.DeepCopy()
	invalidTemplate.Spec.Template.Spec.ResourceGraphDefinitionRef = nil
	if _, err := validator.ValidateCreate(context.Background(), invalidTemplate); err == nil || !apierrors.IsInvalid(err) {
		t.Fatalf("ValidateCreate() expected Invalid error, got %v", err)
	}

	updatedTemplate := validTemplate.DeepCopy()
	updatedTemplate.Spec.Template.Spec.ResourceGraphDefinitionRef = nil
	updatedTemplate.Spec.Template.Spec.ExternalBackend = &Kany8sControlPlaneExternalBackendSpec{
		APIVersion: "controlplane.example.com/v1alpha1",
		Kind:       "ExampleControlPlane",
	}
	if _, err := validator.ValidateUpdate(context.Background(), validTemplate, updatedTemplate); err == nil {
		t.Fatalf("ValidateUpdate() expected immutable backend error")
	} else if !strings.Contains(err.Error(), "backend type is immutable") {
		t.Fatalf("ValidateUpdate() error = %q, want to contain %q", err.Error(), "backend type is immutable")
	}
}
