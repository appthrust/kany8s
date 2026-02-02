package infrastructure

import (
	"context"
	"testing"

	infrastructurev1alpha1 "github.com/reoring/kany8s/api/infrastructure/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestKany8sClusterReconciler_SetsReadyConditionTrue(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := infrastructurev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Kany8sCluster scheme: %v", err)
	}

	kc := &infrastructurev1alpha1.Kany8sCluster{ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"}}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kc).WithStatusSubresource(kc).Build()
	r := &Kany8sClusterReconciler{Client: c, Scheme: scheme}

	ctx := context.Background()
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "demo", Namespace: "default"}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got := &infrastructurev1alpha1.Kany8sCluster{}
	if err := c.Get(ctx, client.ObjectKey{Name: "demo", Namespace: "default"}, got); err != nil {
		t.Fatalf("get Kany8sCluster: %v", err)
	}

	cond := meta.FindStatusCondition(got.Status.Conditions, "Ready")
	if cond == nil {
		t.Fatalf("expected Ready condition")
	}
	if cond.Status != metav1.ConditionTrue {
		t.Fatalf("Ready condition status = %q, want %q", cond.Status, metav1.ConditionTrue)
	}
}

func TestKany8sClusterReconciler_SetsInitializationProvisionedTrue(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := infrastructurev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Kany8sCluster scheme: %v", err)
	}

	kc := &infrastructurev1alpha1.Kany8sCluster{ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"}}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kc).WithStatusSubresource(kc).Build()
	r := &Kany8sClusterReconciler{Client: c, Scheme: scheme}

	ctx := context.Background()
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "demo", Namespace: "default"}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got := &infrastructurev1alpha1.Kany8sCluster{}
	if err := c.Get(ctx, client.ObjectKey{Name: "demo", Namespace: "default"}, got); err != nil {
		t.Fatalf("get Kany8sCluster: %v", err)
	}

	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(got)
	if err != nil {
		t.Fatalf("to unstructured: %v", err)
	}

	provisioned, found, err := unstructured.NestedBool(u, "status", "initialization", "provisioned")
	if err != nil {
		t.Fatalf("get status.initialization.provisioned: %v", err)
	}
	if !found {
		t.Fatalf("expected status.initialization.provisioned to be set")
	}
	if !provisioned {
		t.Fatalf("status.initialization.provisioned = false, want true")
	}
}

func TestKany8sClusterReconciler_CreatesKroInstanceWhenResourceGraphDefinitionRefIsSet(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := infrastructurev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Kany8sCluster scheme: %v", err)
	}

	rgdGVK := schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "ResourceGraphDefinition"}
	instanceGVK := schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "DemoInfra"}

	scheme.AddKnownTypeWithName(rgdGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(rgdGVK.GroupVersion().WithKind("ResourceGraphDefinitionList"), &unstructured.UnstructuredList{})
	scheme.AddKnownTypeWithName(instanceGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(instanceGVK.GroupVersion().WithKind("DemoInfraList"), &unstructured.UnstructuredList{})

	kc := &infrastructurev1alpha1.Kany8sCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"},
		Spec: infrastructurev1alpha1.Kany8sClusterSpec{
			ResourceGraphDefinitionRef: &infrastructurev1alpha1.ResourceGraphDefinitionReference{Name: "demo-infra"},
		},
	}

	rgd := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": rgdGVK.GroupVersion().String(),
		"kind":       rgdGVK.Kind,
		"metadata": map[string]any{
			"name": "demo-infra",
		},
		"spec": map[string]any{
			"schema": map[string]any{
				"apiVersion": "v1alpha1",
				"kind":       instanceGVK.Kind,
			},
		},
	}}
	rgd.SetGroupVersionKind(rgdGVK)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kc, rgd).WithStatusSubresource(kc).Build()
	r := &Kany8sClusterReconciler{Client: c, Scheme: scheme}

	ctx := context.Background()
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "demo", Namespace: "default"}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got := &unstructured.Unstructured{}
	got.SetGroupVersionKind(instanceGVK)
	if err := c.Get(ctx, client.ObjectKey{Name: "demo", Namespace: "default"}, got); err != nil {
		t.Fatalf("get kro instance: %v", err)
	}
	if got.GetName() != "demo" {
		t.Fatalf("kro instance name = %q, want %q", got.GetName(), "demo")
	}
	if got.GetNamespace() != "default" {
		t.Fatalf("kro instance namespace = %q, want %q", got.GetNamespace(), "default")
	}
}
