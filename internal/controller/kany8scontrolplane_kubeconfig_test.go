package controller

import (
	"context"
	"testing"

	controlplanev1alpha1 "github.com/reoring/kany8s/api/v1alpha1"
	"github.com/reoring/kany8s/internal/kubeconfig"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestKany8sControlPlaneReconciler_CreatesKubeconfigSecretFromKroInstanceStatus(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add client-go scheme: %v", err)
	}
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
			Name:      demoName,
			Namespace: demoNamespace,
			UID:       types.UID("00000000-0000-0000-0000-000000000000"),
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

	source := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "provider-kubeconfig",
			Namespace: "default",
		},
		Data: map[string][]byte{
			kubeconfig.DataKey: []byte("kubeconfig-bytes"),
		},
	}

	instance := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": instanceGVK.GroupVersion().String(),
		"kind":       instanceGVK.Kind,
		"metadata": map[string]any{
			"name":      demoName,
			"namespace": demoNamespace,
		},
		"spec": map[string]any{
			"version": "1.34",
		},
		"status": map[string]any{
			"ready":    true,
			"endpoint": "https://api.demo.example.com:6443",
			"kubeconfigSecretRef": map[string]any{
				"name":      "provider-kubeconfig",
				"namespace": demoNamespace,
			},
		},
	}}
	instance.SetGroupVersionKind(instanceGVK)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cp, rgd, instance, source).WithStatusSubresource(cp).Build()
	r := &Kany8sControlPlaneReconciler{Client: c, Scheme: scheme}

	ctx := context.Background()
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: demoName, Namespace: demoNamespace}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got := &corev1.Secret{}
	if err := c.Get(ctx, client.ObjectKey{Name: "demo-kubeconfig", Namespace: demoNamespace}, got); err != nil {
		t.Fatalf("get kubeconfig secret: %v", err)
	}
	if got.Type != kubeconfig.SecretType {
		t.Fatalf("secret type = %q, want %q", got.Type, kubeconfig.SecretType)
	}
	if got.Labels[kubeconfig.ClusterNameLabelKey] != demoName {
		t.Fatalf("secret label %q = %q, want %q", kubeconfig.ClusterNameLabelKey, got.Labels[kubeconfig.ClusterNameLabelKey], demoName)
	}
	if string(got.Data[kubeconfig.DataKey]) != "kubeconfig-bytes" {
		t.Fatalf("secret data[%q] = %q, want %q", kubeconfig.DataKey, string(got.Data[kubeconfig.DataKey]), "kubeconfig-bytes")
	}

	var ownerFound bool
	for _, ref := range got.OwnerReferences {
		if ref.APIVersion != "controlplane.cluster.x-k8s.io/v1alpha1" {
			continue
		}
		if ref.Kind != "Kany8sControlPlane" {
			continue
		}
		if ref.Name != demoName {
			continue
		}
		ownerFound = true
		if ref.UID != cp.UID {
			t.Fatalf("owner ref uid = %q, want %q", ref.UID, cp.UID)
		}
		if ref.Controller == nil || !*ref.Controller {
			t.Fatalf("owner ref controller = %v, want true", ref.Controller)
		}
		break
	}
	if !ownerFound {
		t.Fatalf("kubeconfig secret missing controller owner reference")
	}
}

func TestKany8sControlPlaneReconciler_UpdatesKubeconfigSecretWhenSourceChanges(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add client-go scheme: %v", err)
	}
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
			Name:      demoName,
			Namespace: demoNamespace,
			UID:       types.UID("00000000-0000-0000-0000-000000000000"),
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

	source := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "provider-kubeconfig",
			Namespace: "default",
		},
		Data: map[string][]byte{
			kubeconfig.DataKey: []byte("kubeconfig-v1"),
		},
	}

	instance := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": instanceGVK.GroupVersion().String(),
		"kind":       instanceGVK.Kind,
		"metadata": map[string]any{
			"name":      demoName,
			"namespace": demoNamespace,
		},
		"spec": map[string]any{
			"version": "1.34",
		},
		"status": map[string]any{
			"ready": true,
			"kubeconfigSecretRef": map[string]any{
				"name": "provider-kubeconfig",
				// namespace intentionally omitted to ensure it defaults.
			},
		},
	}}
	instance.SetGroupVersionKind(instanceGVK)

	// Seed an existing target secret with incorrect metadata + data.
	existingTarget, err := kubeconfig.NewSecret(demoName, demoNamespace, []byte("old"))
	if err != nil {
		t.Fatalf("NewSecret: %v", err)
	}
	existingTarget.Type = corev1.SecretTypeOpaque
	delete(existingTarget.Labels, kubeconfig.ClusterNameLabelKey)
	delete(existingTarget.Data, kubeconfig.DataKey)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cp, rgd, instance, source, existingTarget).WithStatusSubresource(cp).Build()
	r := &Kany8sControlPlaneReconciler{Client: c, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: demoName, Namespace: demoNamespace}}
	_, err = r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	target := &corev1.Secret{}
	if err := c.Get(ctx, client.ObjectKey{Name: "demo-kubeconfig", Namespace: demoNamespace}, target); err != nil {
		t.Fatalf("get kubeconfig secret: %v", err)
	}
	if string(target.Data[kubeconfig.DataKey]) != "kubeconfig-v1" {
		t.Fatalf("secret data[%q] = %q, want %q", kubeconfig.DataKey, string(target.Data[kubeconfig.DataKey]), "kubeconfig-v1")
	}
	if target.Type != kubeconfig.SecretType {
		t.Fatalf("secret type = %q, want %q", target.Type, kubeconfig.SecretType)
	}
	if target.Labels[kubeconfig.ClusterNameLabelKey] != demoName {
		t.Fatalf("secret label %q = %q, want %q", kubeconfig.ClusterNameLabelKey, target.Labels[kubeconfig.ClusterNameLabelKey], demoName)
	}

	// Update the source kubeconfig and ensure the target secret follows.
	patchedSource := &corev1.Secret{}
	if err := c.Get(ctx, client.ObjectKey{Name: "provider-kubeconfig", Namespace: demoNamespace}, patchedSource); err != nil {
		t.Fatalf("get source secret: %v", err)
	}
	patchedSource.Data[kubeconfig.DataKey] = []byte("kubeconfig-v2")
	if err := c.Update(ctx, patchedSource); err != nil {
		t.Fatalf("update source secret: %v", err)
	}

	_, err = r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("reconcile after source update: %v", err)
	}

	target2 := &corev1.Secret{}
	if err := c.Get(ctx, client.ObjectKey{Name: "demo-kubeconfig", Namespace: demoNamespace}, target2); err != nil {
		t.Fatalf("get kubeconfig secret after update: %v", err)
	}
	if string(target2.Data[kubeconfig.DataKey]) != "kubeconfig-v2" {
		t.Fatalf("secret data[%q] after update = %q, want %q", kubeconfig.DataKey, string(target2.Data[kubeconfig.DataKey]), "kubeconfig-v2")
	}
}
