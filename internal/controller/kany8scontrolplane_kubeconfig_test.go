package controller

import (
	"context"
	"strings"
	"testing"
	"time"

	controlplanev1alpha1 "github.com/reoring/kany8s/api/v1alpha1"
	"github.com/reoring/kany8s/internal/constants"
	"github.com/reoring/kany8s/internal/kubeconfig"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	validKubeconfigV1 = `apiVersion: v1
kind: Config
clusters:
- name: demo
  cluster:
    server: https://api.demo.example.com:6443
    certificate-authority-data: ZHVtbXk=
users:
- name: demo
  user:
    token: dummy-v1
contexts:
- name: demo
  context:
    cluster: demo
    user: demo
current-context: demo
`

	validKubeconfigV2 = `apiVersion: v1
kind: Config
clusters:
- name: demo
  cluster:
    server: https://api.demo.example.com:6443
    certificate-authority-data: ZHVtbXk=
users:
- name: demo
  user:
    token: dummy-v2
contexts:
- name: demo
  context:
    cluster: demo
    user: demo
current-context: demo
`
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
			kubeconfig.DataKey: []byte(validKubeconfigV1),
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
	if string(got.Data[kubeconfig.DataKey]) != validKubeconfigV1 {
		t.Fatalf("secret data[%q] = %q, want %q", kubeconfig.DataKey, string(got.Data[kubeconfig.DataKey]), validKubeconfigV1)
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
			kubeconfig.DataKey: []byte(validKubeconfigV1),
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
	if string(target.Data[kubeconfig.DataKey]) != validKubeconfigV1 {
		t.Fatalf("secret data[%q] = %q, want %q", kubeconfig.DataKey, string(target.Data[kubeconfig.DataKey]), validKubeconfigV1)
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
	patchedSource.Data[kubeconfig.DataKey] = []byte(validKubeconfigV2)
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
	if string(target2.Data[kubeconfig.DataKey]) != validKubeconfigV2 {
		t.Fatalf("secret data[%q] after update = %q, want %q", kubeconfig.DataKey, string(target2.Data[kubeconfig.DataKey]), validKubeconfigV2)
	}
}

func TestKany8sControlPlaneReconciler_RequeuesWhenKubeconfigSourceSecretIsNotFound(t *testing.T) {
	t.Parallel()

	const reasonSourceSecretNotFound = "KubeconfigSourceSecretNotFound"

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
	// Seed the kubeconfig condition as True so we can ensure it does not stay stale.
	cp.Status.Conditions = []metav1.Condition{{
		Type:               conditionTypeKubeconfigSecretReconciled,
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            "kubeconfig secret reconciled",
		ObservedGeneration: cp.Generation,
	}}

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

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cp, rgd, instance).WithStatusSubresource(cp).Build()
	recorder := record.NewFakeRecorder(16)
	r := &Kany8sControlPlaneReconciler{Client: c, Scheme: scheme, Recorder: recorder}

	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: demoName, Namespace: demoNamespace}}

	res, err := r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.RequeueAfter != constants.ControlPlaneNotReadyRequeueAfter {
		t.Fatalf("RequeueAfter = %s, want %s", res.RequeueAfter, constants.ControlPlaneNotReadyRequeueAfter)
	}

	gotCP := &controlplanev1alpha1.Kany8sControlPlane{}
	if err := c.Get(ctx, client.ObjectKey{Name: demoName, Namespace: demoNamespace}, gotCP); err != nil {
		t.Fatalf("get control plane: %v", err)
	}
	cond := meta.FindStatusCondition(gotCP.Status.Conditions, conditionTypeKubeconfigSecretReconciled)
	if cond == nil {
		t.Fatalf("expected %s condition", conditionTypeKubeconfigSecretReconciled)
	}
	if cond.Status != metav1.ConditionFalse {
		t.Fatalf("condition status = %q, want %q", cond.Status, metav1.ConditionFalse)
	}
	if cond.Reason != reasonSourceSecretNotFound {
		t.Fatalf("condition reason = %q, want %q", cond.Reason, reasonSourceSecretNotFound)
	}
	if !strings.Contains(cond.Message, "waiting for source secret") {
		t.Fatalf("condition message = %q, want to contain %q", cond.Message, "waiting for source secret")
	}

	select {
	case evt := <-recorder.Events:
		if !strings.Contains(evt, reasonSourceSecretNotFound) {
			t.Fatalf("event = %q, want to contain %q", evt, reasonSourceSecretNotFound)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("expected an event to be recorded")
	}

	target := &corev1.Secret{}
	if err := c.Get(ctx, client.ObjectKey{Name: "demo-kubeconfig", Namespace: demoNamespace}, target); err == nil || !apierrors.IsNotFound(err) {
		t.Fatalf("expected target secret to not exist yet")
	}

	// Subsequent reconciles while still NotFound should not spam events.
	resAgain, err := r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("reconcile again: %v", err)
	}
	if resAgain.RequeueAfter != constants.ControlPlaneNotReadyRequeueAfter {
		t.Fatalf("RequeueAfter (2nd) = %s, want %s", resAgain.RequeueAfter, constants.ControlPlaneNotReadyRequeueAfter)
	}
	select {
	case evt := <-recorder.Events:
		t.Fatalf("unexpected event recorded: %q", evt)
	default:
	}

	source := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "provider-kubeconfig",
			Namespace: demoNamespace,
		},
		Data: map[string][]byte{
			kubeconfig.DataKey: []byte(validKubeconfigV1),
		},
	}
	if err := c.Create(ctx, source); err != nil {
		t.Fatalf("create source secret: %v", err)
	}

	res2, err := r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("reconcile after creating source secret: %v", err)
	}
	if res2.RequeueAfter != 0 {
		t.Fatalf("RequeueAfter after source creation = %s, want 0", res2.RequeueAfter)
	}

	if err := c.Get(ctx, client.ObjectKey{Name: "demo-kubeconfig", Namespace: demoNamespace}, target); err != nil {
		t.Fatalf("get kubeconfig secret: %v", err)
	}
	if string(target.Data[kubeconfig.DataKey]) != validKubeconfigV1 {
		t.Fatalf("secret data[%q] = %q, want %q", kubeconfig.DataKey, string(target.Data[kubeconfig.DataKey]), validKubeconfigV1)
	}
}

func TestKany8sControlPlaneReconciler_RequeuesWhenKubeconfigSourceSecretIsMissingDataKey(t *testing.T) {
	t.Parallel()

	const reasonMissingDataKey = "KubeconfigSourceSecretDataMissing"

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

	// Source Secret exists but is not populated yet.
	source := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "provider-kubeconfig", Namespace: demoNamespace}}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cp, rgd, instance, source).WithStatusSubresource(cp).Build()
	recorder := record.NewFakeRecorder(16)
	r := &Kany8sControlPlaneReconciler{Client: c, Scheme: scheme, Recorder: recorder}

	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: demoName, Namespace: demoNamespace}}

	res1, err := r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res1.RequeueAfter != constants.ControlPlaneNotReadyRequeueAfter {
		t.Fatalf("RequeueAfter = %s, want %s", res1.RequeueAfter, constants.ControlPlaneNotReadyRequeueAfter)
	}

	target := &corev1.Secret{}
	if err := c.Get(ctx, client.ObjectKey{Name: "demo-kubeconfig", Namespace: demoNamespace}, target); err == nil || !apierrors.IsNotFound(err) {
		t.Fatalf("expected target secret to not exist yet")
	}

	gotCP := &controlplanev1alpha1.Kany8sControlPlane{}
	if err := c.Get(ctx, client.ObjectKey{Name: demoName, Namespace: demoNamespace}, gotCP); err != nil {
		t.Fatalf("get control plane: %v", err)
	}
	cond := meta.FindStatusCondition(gotCP.Status.Conditions, conditionTypeKubeconfigSecretReconciled)
	if cond == nil {
		t.Fatalf("expected %s condition", conditionTypeKubeconfigSecretReconciled)
	}
	if cond.Status != metav1.ConditionFalse {
		t.Fatalf("condition status = %q, want %q", cond.Status, metav1.ConditionFalse)
	}
	if cond.Reason != reasonMissingDataKey {
		t.Fatalf("condition reason = %q, want %q", cond.Reason, reasonMissingDataKey)
	}
	if !strings.Contains(cond.Message, "missing data") {
		t.Fatalf("condition message = %q, want to contain %q", cond.Message, "missing data")
	}

	select {
	case evt := <-recorder.Events:
		if !strings.Contains(evt, reasonMissingDataKey) {
			t.Fatalf("event = %q, want to contain %q", evt, reasonMissingDataKey)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("expected an event to be recorded")
	}

	// Subsequent reconciles while still missing data should not spam events.
	res2, err := r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("reconcile again: %v", err)
	}
	if res2.RequeueAfter != constants.ControlPlaneNotReadyRequeueAfter {
		t.Fatalf("RequeueAfter (2nd) = %s, want %s", res2.RequeueAfter, constants.ControlPlaneNotReadyRequeueAfter)
	}
	select {
	case evt := <-recorder.Events:
		t.Fatalf("unexpected event recorded: %q", evt)
	default:
	}

	// Populate the key and ensure we reconcile the target Secret.
	patchedSource := &corev1.Secret{}
	if err := c.Get(ctx, client.ObjectKey{Name: "provider-kubeconfig", Namespace: demoNamespace}, patchedSource); err != nil {
		t.Fatalf("get source secret: %v", err)
	}
	if patchedSource.Data == nil {
		patchedSource.Data = map[string][]byte{}
	}
	patchedSource.Data[kubeconfig.DataKey] = []byte(validKubeconfigV1)
	if err := c.Update(ctx, patchedSource); err != nil {
		t.Fatalf("update source secret: %v", err)
	}

	res3, err := r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("reconcile after populating source: %v", err)
	}
	if res3.RequeueAfter != 0 {
		t.Fatalf("RequeueAfter after populate = %s, want 0", res3.RequeueAfter)
	}
	if err := c.Get(ctx, client.ObjectKey{Name: "demo-kubeconfig", Namespace: demoNamespace}, target); err != nil {
		t.Fatalf("get kubeconfig secret: %v", err)
	}
	if string(target.Data[kubeconfig.DataKey]) != validKubeconfigV1 {
		t.Fatalf("secret data[%q] = %q, want %q", kubeconfig.DataKey, string(target.Data[kubeconfig.DataKey]), validKubeconfigV1)
	}
}

func TestKany8sControlPlaneReconciler_DoesNotOverwriteTargetSecretWithInvalidKubeconfig(t *testing.T) {
	t.Parallel()

	const reasonInvalidKubeconfig = "InvalidKubeconfig"

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

	// Seed an existing target secret with a valid kubeconfig.
	existingTarget, err := kubeconfig.NewSecret(demoName, demoNamespace, []byte(validKubeconfigV1))
	if err != nil {
		t.Fatalf("NewSecret: %v", err)
	}

	source := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "provider-kubeconfig",
			Namespace: demoNamespace,
		},
		Data: map[string][]byte{
			kubeconfig.DataKey: []byte("not a kubeconfig"),
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cp, rgd, instance, source, existingTarget).WithStatusSubresource(cp).Build()
	recorder := record.NewFakeRecorder(16)
	r := &Kany8sControlPlaneReconciler{Client: c, Scheme: scheme, Recorder: recorder}

	ctx := context.Background()
	res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: demoName, Namespace: demoNamespace}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.RequeueAfter != constants.ControlPlaneNotReadyRequeueAfter {
		t.Fatalf("RequeueAfter = %s, want %s", res.RequeueAfter, constants.ControlPlaneNotReadyRequeueAfter)
	}

	target := &corev1.Secret{}
	if err := c.Get(ctx, client.ObjectKey{Name: "demo-kubeconfig", Namespace: demoNamespace}, target); err != nil {
		t.Fatalf("get target kubeconfig secret: %v", err)
	}
	if string(target.Data[kubeconfig.DataKey]) != validKubeconfigV1 {
		t.Fatalf("target secret data[%q] = %q, want %q", kubeconfig.DataKey, string(target.Data[kubeconfig.DataKey]), validKubeconfigV1)
	}

	gotCP := &controlplanev1alpha1.Kany8sControlPlane{}
	if err := c.Get(ctx, client.ObjectKey{Name: demoName, Namespace: demoNamespace}, gotCP); err != nil {
		t.Fatalf("get control plane: %v", err)
	}

	cond := meta.FindStatusCondition(gotCP.Status.Conditions, conditionTypeKubeconfigSecretReconciled)
	if cond == nil {
		t.Fatalf("expected %s condition", conditionTypeKubeconfigSecretReconciled)
	}
	if cond.Status != metav1.ConditionFalse {
		t.Fatalf("condition status = %q, want %q", cond.Status, metav1.ConditionFalse)
	}
	if cond.Reason != reasonInvalidKubeconfig {
		t.Fatalf("condition reason = %q, want %q", cond.Reason, reasonInvalidKubeconfig)
	}
	if !strings.Contains(cond.Message, "invalid kubeconfig") {
		t.Fatalf("condition message = %q, want to contain %q", cond.Message, "invalid kubeconfig")
	}

	select {
	case evt := <-recorder.Events:
		if !strings.Contains(evt, reasonInvalidKubeconfig) {
			t.Fatalf("event = %q, want to contain %q", evt, reasonInvalidKubeconfig)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("expected an event to be recorded")
	}
}

func TestKany8sControlPlaneReconciler_SurfacesKubeconfigSourceSecretGetErrorViaConditionAndEvent(t *testing.T) {
	t.Parallel()

	const (
		kubeconfigCondType          = "KubeconfigSecretReconciled"
		reasonSourceSecretGetFailed = "KubeconfigSourceSecretGetFailed"
	)

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

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cp, rgd, instance).WithStatusSubresource(cp).Build()
	wantKey := client.ObjectKey{Name: "provider-kubeconfig", Namespace: demoNamespace}
	getErr := apierrors.NewForbidden(schema.GroupResource{Group: "", Resource: "secrets"}, wantKey.Name, nil)
	wrappedClient := &getErrorClient{Client: c, Key: wantKey, Err: getErr}

	recorder := record.NewFakeRecorder(16)
	r := &Kany8sControlPlaneReconciler{Client: wrappedClient, Scheme: scheme, Recorder: recorder}

	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: demoName, Namespace: demoNamespace}}

	res, err := r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.RequeueAfter != constants.ControlPlaneNotReadyRequeueAfter {
		t.Fatalf("RequeueAfter = %s, want %s", res.RequeueAfter, constants.ControlPlaneNotReadyRequeueAfter)
	}

	got := &controlplanev1alpha1.Kany8sControlPlane{}
	if err := c.Get(ctx, client.ObjectKey{Name: demoName, Namespace: demoNamespace}, got); err != nil {
		t.Fatalf("get control plane: %v", err)
	}

	cond := meta.FindStatusCondition(got.Status.Conditions, kubeconfigCondType)
	if cond == nil {
		t.Fatalf("expected %s condition", kubeconfigCondType)
	}
	if cond.Status != metav1.ConditionFalse {
		t.Fatalf("condition status = %q, want %q", cond.Status, metav1.ConditionFalse)
	}
	if cond.Reason != reasonSourceSecretGetFailed {
		t.Fatalf("condition reason = %q, want %q", cond.Reason, reasonSourceSecretGetFailed)
	}
	if !strings.Contains(cond.Message, "get source secret") {
		t.Fatalf("condition message = %q, want to contain %q", cond.Message, "get source secret")
	}

	select {
	case evt := <-recorder.Events:
		if !strings.Contains(evt, reasonSourceSecretGetFailed) {
			t.Fatalf("event = %q, want to contain %q", evt, reasonSourceSecretGetFailed)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("expected an event to be recorded")
	}
}

type getErrorClient struct {
	client.Client
	Key client.ObjectKey
	Err error
}

func (c *getErrorClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if key == c.Key {
		return c.Err
	}
	return c.Client.Get(ctx, key, obj, opts...)
}
