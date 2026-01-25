package controller

import (
	"context"
	"strings"
	"testing"
	"time"

	controlplanev1alpha1 "github.com/reoring/kany8s/api/v1alpha1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const provisioningReason = "Provisioning"

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

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cp, rgd).WithStatusSubresource(cp).Build()

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

func TestKany8sControlPlaneReconciler_SetsOwnerReferenceOnKroInstance(t *testing.T) {
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

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cp, rgd).WithStatusSubresource(cp).Build()
	r := &Kany8sControlPlaneReconciler{Client: c, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "demo", Namespace: "default"}}
	_, err := r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got := &unstructured.Unstructured{}
	got.SetGroupVersionKind(instanceGVK)
	if err := c.Get(ctx, client.ObjectKey{Name: "demo", Namespace: "default"}, got); err != nil {
		t.Fatalf("get kro instance: %v", err)
	}

	var found bool
	for _, ref := range got.GetOwnerReferences() {
		if ref.APIVersion != "controlplane.cluster.x-k8s.io/v1alpha1" {
			continue
		}
		if ref.Kind != "Kany8sControlPlane" {
			continue
		}
		if ref.Name != "demo" {
			continue
		}

		found = true
		if ref.UID != cp.UID {
			t.Fatalf("owner ref uid = %q, want %q", ref.UID, cp.UID)
		}
		if ref.Controller == nil || !*ref.Controller {
			t.Fatalf("owner ref controller = %v, want true", ref.Controller)
		}
		break
	}
	if !found {
		t.Fatalf("kro instance missing controller owner reference")
	}
}

func TestKany8sControlPlaneReconciler_BuildsKroInstanceSpec(t *testing.T) {
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
			KroSpec: &apiextensionsv1.JSON{Raw: []byte(`{"region":"ap-northeast-1","version":"0.0"}`)},
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

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cp, rgd).WithStatusSubresource(cp).Build()
	r := &Kany8sControlPlaneReconciler{Client: c, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "demo", Namespace: "default"}}

	_, err := r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got := &unstructured.Unstructured{}
	got.SetGroupVersionKind(instanceGVK)
	if err := c.Get(ctx, client.ObjectKey{Name: "demo", Namespace: "default"}, got); err != nil {
		t.Fatalf("get kro instance: %v", err)
	}

	spec, found, err := unstructured.NestedMap(got.Object, "spec")
	if err != nil {
		t.Fatalf("get instance spec: %v", err)
	}
	if !found {
		t.Fatalf("instance spec not found")
	}
	if spec["region"] != "ap-northeast-1" {
		t.Fatalf("instance spec.region = %v, want %q", spec["region"], "ap-northeast-1")
	}
	if spec["version"] != "1.34" {
		t.Fatalf("instance spec.version = %v, want %q", spec["version"], "1.34")
	}

	if err := unstructured.SetNestedField(got.Object, "9.99", "spec", "version"); err != nil {
		t.Fatalf("set drifted instance spec.version: %v", err)
	}
	if err := unstructured.SetNestedField(got.Object, "us-west-2", "spec", "region"); err != nil {
		t.Fatalf("set drifted instance spec.region: %v", err)
	}
	if err := c.Update(ctx, got); err != nil {
		t.Fatalf("update drifted instance: %v", err)
	}

	_, err = r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("reconcile after drift: %v", err)
	}

	got2 := &unstructured.Unstructured{}
	got2.SetGroupVersionKind(instanceGVK)
	if err := c.Get(ctx, client.ObjectKey{Name: "demo", Namespace: "default"}, got2); err != nil {
		t.Fatalf("get kro instance after drift: %v", err)
	}

	spec2, found, err := unstructured.NestedMap(got2.Object, "spec")
	if err != nil {
		t.Fatalf("get instance spec after drift: %v", err)
	}
	if !found {
		t.Fatalf("instance spec not found after drift")
	}
	if spec2["region"] != "ap-northeast-1" {
		t.Fatalf("instance spec.region after drift = %v, want %q", spec2["region"], "ap-northeast-1")
	}
	if spec2["version"] != "1.34" {
		t.Fatalf("instance spec.version after drift = %v, want %q", spec2["version"], "1.34")
	}
}

func TestKany8sControlPlaneReconciler_SetsControlPlaneEndpointFromKroInstanceStatus(t *testing.T) {
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

	instance := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": instanceGVK.GroupVersion().String(),
		"kind":       instanceGVK.Kind,
		"metadata": map[string]any{
			"name":      "demo",
			"namespace": "default",
		},
		"spec": map[string]any{
			"version": "1.34",
		},
		"status": map[string]any{
			"ready":    true,
			"endpoint": "https://api.demo.example.com:6443",
		},
	}}
	instance.SetGroupVersionKind(instanceGVK)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cp, rgd, instance).WithStatusSubresource(cp).Build()
	r := &Kany8sControlPlaneReconciler{Client: c, Scheme: scheme}

	ctx := context.Background()
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "demo", Namespace: "default"}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got := &controlplanev1alpha1.Kany8sControlPlane{}
	if err := c.Get(ctx, client.ObjectKey{Name: "demo", Namespace: "default"}, got); err != nil {
		t.Fatalf("get control plane: %v", err)
	}
	if got.Spec.ControlPlaneEndpoint.Host != "api.demo.example.com" {
		t.Fatalf("control plane endpoint host = %q, want %q", got.Spec.ControlPlaneEndpoint.Host, "api.demo.example.com")
	}
	if got.Spec.ControlPlaneEndpoint.Port != 6443 {
		t.Fatalf("control plane endpoint port = %d, want %d", got.Spec.ControlPlaneEndpoint.Port, 6443)
	}
}

func TestKany8sControlPlaneReconciler_SetsControlPlaneInitializedWhenEndpointIsSet(t *testing.T) {
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

	instance := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": instanceGVK.GroupVersion().String(),
		"kind":       instanceGVK.Kind,
		"metadata": map[string]any{
			"name":      "demo",
			"namespace": "default",
		},
		"spec": map[string]any{
			"version": "1.34",
		},
		"status": map[string]any{
			"ready":    true,
			"endpoint": "https://api.demo.example.com:6443",
		},
	}}
	instance.SetGroupVersionKind(instanceGVK)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cp, rgd, instance).WithStatusSubresource(cp).Build()
	r := &Kany8sControlPlaneReconciler{Client: c, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "demo", Namespace: "default"}}
	_, err := r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got := &controlplanev1alpha1.Kany8sControlPlane{}
	if err := c.Get(ctx, client.ObjectKey{Name: "demo", Namespace: "default"}, got); err != nil {
		t.Fatalf("get control plane: %v", err)
	}
	if !got.Status.Initialization.ControlPlaneInitialized {
		t.Fatalf("control plane initialized = %v, want %v", got.Status.Initialization.ControlPlaneInitialized, true)
	}

	instance2 := &unstructured.Unstructured{}
	instance2.SetGroupVersionKind(instanceGVK)
	if err := c.Get(ctx, client.ObjectKey{Name: "demo", Namespace: "default"}, instance2); err != nil {
		t.Fatalf("get kro instance: %v", err)
	}
	if err := unstructured.SetNestedField(instance2.Object, "", "status", "endpoint"); err != nil {
		t.Fatalf("clear instance status.endpoint: %v", err)
	}
	if err := c.Update(ctx, instance2); err != nil {
		t.Fatalf("update kro instance: %v", err)
	}

	_, err = r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("reconcile after endpoint cleared: %v", err)
	}

	got2 := &controlplanev1alpha1.Kany8sControlPlane{}
	if err := c.Get(ctx, client.ObjectKey{Name: "demo", Namespace: "default"}, got2); err != nil {
		t.Fatalf("get control plane after endpoint cleared: %v", err)
	}
	if !got2.Status.Initialization.ControlPlaneInitialized {
		t.Fatalf("control plane initialized after endpoint cleared = %v, want %v", got2.Status.Initialization.ControlPlaneInitialized, true)
	}
}

func TestKany8sControlPlaneReconciler_RequeuesWhenRGDNotFound(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := controlplanev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Kany8sControlPlane scheme: %v", err)
	}

	rgdGVK := schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "ResourceGraphDefinition"}
	scheme.AddKnownTypeWithName(rgdGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(rgdGVK.GroupVersion().WithKind("ResourceGraphDefinitionList"), &unstructured.UnstructuredList{})

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

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cp).WithStatusSubresource(cp).Build()

	recorder := record.NewFakeRecorder(16)
	r := &Kany8sControlPlaneReconciler{Client: c, Scheme: scheme, Recorder: recorder}

	ctx := context.Background()
	res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "demo", Namespace: "default"}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.RequeueAfter == 0 {
		t.Fatalf("expected RequeueAfter > 0")
	}

	got := &controlplanev1alpha1.Kany8sControlPlane{}
	if err := c.Get(ctx, client.ObjectKey{Name: "demo", Namespace: "default"}, got); err != nil {
		t.Fatalf("get control plane: %v", err)
	}

	cond := meta.FindStatusCondition(got.Status.Conditions, "ResourceGraphDefinitionResolved")
	if cond == nil {
		t.Fatalf("expected ResourceGraphDefinitionResolved condition")
	}
	if cond.Status != metav1.ConditionFalse {
		t.Fatalf("condition status = %q, want %q", cond.Status, metav1.ConditionFalse)
	}
	if cond.Reason != "ResourceGraphDefinitionNotFound" {
		t.Fatalf("condition reason = %q, want %q", cond.Reason, "ResourceGraphDefinitionNotFound")
	}
	if !strings.Contains(cond.Message, "eks-control-plane") {
		t.Fatalf("condition message = %q, want to contain %q", cond.Message, "eks-control-plane")
	}

	select {
	case evt := <-recorder.Events:
		if !strings.Contains(evt, "ResourceGraphDefinitionNotFound") {
			t.Fatalf("event = %q, want to contain %q", evt, "ResourceGraphDefinitionNotFound")
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("expected an event to be recorded")
	}
}

func TestKany8sControlPlaneReconciler_RequeuesWhenRGDInvalid(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := controlplanev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Kany8sControlPlane scheme: %v", err)
	}

	rgdGVK := schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "ResourceGraphDefinition"}
	scheme.AddKnownTypeWithName(rgdGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(rgdGVK.GroupVersion().WithKind("ResourceGraphDefinitionList"), &unstructured.UnstructuredList{})

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
				// kind is intentionally missing
			},
		},
	}}
	rgd.SetGroupVersionKind(rgdGVK)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cp, rgd).WithStatusSubresource(cp).Build()

	recorder := record.NewFakeRecorder(16)
	r := &Kany8sControlPlaneReconciler{Client: c, Scheme: scheme, Recorder: recorder}

	ctx := context.Background()
	res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "demo", Namespace: "default"}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.RequeueAfter == 0 {
		t.Fatalf("expected RequeueAfter > 0")
	}

	got := &controlplanev1alpha1.Kany8sControlPlane{}
	if err := c.Get(ctx, client.ObjectKey{Name: "demo", Namespace: "default"}, got); err != nil {
		t.Fatalf("get control plane: %v", err)
	}

	cond := meta.FindStatusCondition(got.Status.Conditions, "ResourceGraphDefinitionResolved")
	if cond == nil {
		t.Fatalf("expected ResourceGraphDefinitionResolved condition")
	}
	if cond.Status != metav1.ConditionFalse {
		t.Fatalf("condition status = %q, want %q", cond.Status, metav1.ConditionFalse)
	}
	if cond.Reason != "ResourceGraphDefinitionInvalid" {
		t.Fatalf("condition reason = %q, want %q", cond.Reason, "ResourceGraphDefinitionInvalid")
	}
	if !strings.Contains(cond.Message, "missing spec.schema.kind") {
		t.Fatalf("condition message = %q, want to contain %q", cond.Message, "missing spec.schema.kind")
	}

	select {
	case evt := <-recorder.Events:
		if !strings.Contains(evt, "ResourceGraphDefinitionInvalid") {
			t.Fatalf("event = %q, want to contain %q", evt, "ResourceGraphDefinitionInvalid")
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("expected an event to be recorded")
	}
}

func TestKany8sControlPlaneReconciler_SetsCreatingConditionAndFailureFieldsWhenNotReady(t *testing.T) {
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

	instance := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": instanceGVK.GroupVersion().String(),
		"kind":       instanceGVK.Kind,
		"metadata": map[string]any{
			"name":      "demo",
			"namespace": "default",
		},
		"spec": map[string]any{
			"version": "1.34",
		},
		"status": map[string]any{
			"ready":   false,
			"reason":  provisioningReason,
			"message": defaultNotReadyMessage,
		},
	}}
	instance.SetGroupVersionKind(instanceGVK)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cp, rgd, instance).WithStatusSubresource(cp).Build()
	r := &Kany8sControlPlaneReconciler{Client: c, Scheme: scheme}

	ctx := context.Background()
	res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "demo", Namespace: "default"}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.RequeueAfter != 15*time.Second {
		t.Fatalf("RequeueAfter = %s, want %s", res.RequeueAfter, 15*time.Second)
	}

	got := &controlplanev1alpha1.Kany8sControlPlane{}
	if err := c.Get(ctx, client.ObjectKey{Name: "demo", Namespace: "default"}, got); err != nil {
		t.Fatalf("get control plane: %v", err)
	}

	creatingCond := meta.FindStatusCondition(got.Status.Conditions, "Creating")
	if creatingCond == nil {
		t.Fatalf("expected Creating condition")
	}
	if creatingCond.Status != metav1.ConditionTrue {
		t.Fatalf("Creating condition status = %q, want %q", creatingCond.Status, metav1.ConditionTrue)
	}
	if creatingCond.Reason != provisioningReason {
		t.Fatalf("Creating condition reason = %q, want %q", creatingCond.Reason, provisioningReason)
	}
	if creatingCond.Message != defaultNotReadyMessage {
		t.Fatalf("Creating condition message = %q, want %q", creatingCond.Message, defaultNotReadyMessage)
	}

	readyCond := meta.FindStatusCondition(got.Status.Conditions, "Ready")
	if readyCond == nil {
		t.Fatalf("expected Ready condition")
	}
	if readyCond.Status != metav1.ConditionFalse {
		t.Fatalf("Ready condition status = %q, want %q", readyCond.Status, metav1.ConditionFalse)
	}
	if readyCond.Reason != provisioningReason {
		t.Fatalf("Ready condition reason = %q, want %q", readyCond.Reason, provisioningReason)
	}
	if readyCond.Message != defaultNotReadyMessage {
		t.Fatalf("Ready condition message = %q, want %q", readyCond.Message, defaultNotReadyMessage)
	}

	if got.Status.FailureReason == nil {
		t.Fatalf("expected failureReason to be set")
	}
	if *got.Status.FailureReason != provisioningReason {
		t.Fatalf("failureReason = %q, want %q", *got.Status.FailureReason, provisioningReason)
	}
	if got.Status.FailureMessage == nil {
		t.Fatalf("expected failureMessage to be set")
	}
	if *got.Status.FailureMessage != defaultNotReadyMessage {
		t.Fatalf("failureMessage = %q, want %q", *got.Status.FailureMessage, defaultNotReadyMessage)
	}
}

func TestKany8sControlPlaneReconciler_SetsReadyConditionAndClearsFailureFieldsWhenReady(t *testing.T) {
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
		Status: controlplanev1alpha1.Kany8sControlPlaneStatus{
			FailureReason:  ptrToString(provisioningReason),
			FailureMessage: ptrToString("waiting"),
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

	instance := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": instanceGVK.GroupVersion().String(),
		"kind":       instanceGVK.Kind,
		"metadata": map[string]any{
			"name":      "demo",
			"namespace": "default",
		},
		"spec": map[string]any{
			"version": "1.34",
		},
		"status": map[string]any{
			"ready":    true,
			"endpoint": "https://api.demo.example.com:6443",
			"reason":   "Ready",
			"message":  "control plane is ready",
		},
	}}
	instance.SetGroupVersionKind(instanceGVK)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cp, rgd, instance).WithStatusSubresource(cp).Build()
	r := &Kany8sControlPlaneReconciler{Client: c, Scheme: scheme}

	ctx := context.Background()
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "demo", Namespace: "default"}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got := &controlplanev1alpha1.Kany8sControlPlane{}
	if err := c.Get(ctx, client.ObjectKey{Name: "demo", Namespace: "default"}, got); err != nil {
		t.Fatalf("get control plane: %v", err)
	}

	readyCond := meta.FindStatusCondition(got.Status.Conditions, "Ready")
	if readyCond == nil {
		t.Fatalf("expected Ready condition")
	}
	if readyCond.Status != metav1.ConditionTrue {
		t.Fatalf("Ready condition status = %q, want %q", readyCond.Status, metav1.ConditionTrue)
	}

	creatingCond := meta.FindStatusCondition(got.Status.Conditions, "Creating")
	if creatingCond != nil {
		t.Fatalf("expected Creating condition to be absent when Ready=true")
	}

	if got.Status.FailureReason != nil {
		t.Fatalf("expected failureReason to be cleared, got %q", *got.Status.FailureReason)
	}
	if got.Status.FailureMessage != nil {
		t.Fatalf("expected failureMessage to be cleared, got %q", *got.Status.FailureMessage)
	}
}

func ptrToString(s string) *string {
	return &s
}
