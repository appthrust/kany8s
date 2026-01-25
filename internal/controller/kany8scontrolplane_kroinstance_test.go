package controller

import (
	"context"
	"testing"

	controlplanev1alpha1 "github.com/reoring/kany8s/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestKany8sControlPlaneReconciler_CreatesKroInstance(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := controlplanev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Kany8sControlPlane scheme: %v", err)
	}

	rgdGVK := schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "ResourceGraphDefinition"}
	instanceGVK := schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "EKSControlPlane"}

	scheme.AddKnownTypeWithName(rgdGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(rgdGVK.GroupVersion().WithKind("ResourceGraphDefinitionList"), &unstructured.UnstructuredList{})
	scheme.AddKnownTypeWithName(instanceGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(instanceGVK.GroupVersion().WithKind("EKSControlPlaneList"), &unstructured.UnstructuredList{})

	cp := &controlplanev1alpha1.Kany8sControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Spec: controlplanev1alpha1.Kany8sControlPlaneSpec{
			Version: "1.34",
			ResourceGraphDefinitionRef: controlplanev1alpha1.ResourceGraphDefinitionReference{
				Name: "eks-control-plane",
			},
		},
	}

	rgd := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": rgdGVK.GroupVersion().String(),
		"kind":       rgdGVK.Kind,
		"metadata": map[string]any{
			"name": "eks-control-plane",
		},
		"spec": map[string]any{
			"schema": map[string]any{
				"apiVersion": "v1alpha1",
				"kind":       instanceGVK.Kind,
			},
		},
	}}
	rgd.SetGroupVersionKind(rgdGVK)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cp, rgd).Build()

	r := &Kany8sControlPlaneReconciler{Client: c, Scheme: scheme}

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: "demo", Namespace: "default"}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got := &unstructured.Unstructured{}
	got.SetGroupVersionKind(instanceGVK)
	if err := c.Get(context.Background(), client.ObjectKey{Name: "demo", Namespace: "default"}, got); err != nil {
		t.Fatalf("get kro instance: %v", err)
	}
	if got.GetName() != "demo" {
		t.Fatalf("kro instance name = %q, want %q", got.GetName(), "demo")
	}
	if got.GetNamespace() != "default" {
		t.Fatalf("kro instance namespace = %q, want %q", got.GetNamespace(), "default")
	}
}
