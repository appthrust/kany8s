package controller

import (
	"context"
	"testing"

	controlplanev1alpha1 "github.com/reoring/kany8s/api/v1alpha1"
	"github.com/reoring/kany8s/internal/constants"
	"github.com/reoring/kany8s/internal/kubeconfig"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestKany8sControlPlaneReconciler_ReconcilesKubeadmBackendAndReflectsStatus(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := controlplanev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add kany8s scheme: %v", err)
	}
	if err := clusterv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add cluster-api scheme: %v", err)
	}

	cp := &controlplanev1alpha1.Kany8sControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      demoName,
			Namespace: demoNamespace,
			UID:       types.UID("00000000-0000-0000-0000-000000000000"),
		},
		Spec: controlplanev1alpha1.Kany8sControlPlaneSpec{
			Version: "v1.34.0",
			Kubeadm: &controlplanev1alpha1.Kany8sControlPlaneKubeadmSpec{
				MachineTemplate: controlplanev1alpha1.Kany8sKubeadmControlPlaneMachineTemplate{
					InfrastructureRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: "infrastructure.cluster.x-k8s.io",
						Kind:     "DockerMachineTemplate",
						Name:     "demo-control-plane",
					},
				},
			},
		},
	}
	ownerCluster := attachOwnerClusterToControlPlane(cp)

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cp, ownerCluster).
		WithStatusSubresource(cp).
		Build()
	r := &Kany8sControlPlaneReconciler{Client: c, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: demoName, Namespace: demoNamespace}}

	res, err := r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.RequeueAfter != constants.ControlPlaneNotReadyRequeueAfter {
		t.Fatalf("RequeueAfter = %s, want %s", res.RequeueAfter, constants.ControlPlaneNotReadyRequeueAfter)
	}

	backend := &controlplanev1alpha1.Kany8sKubeadmControlPlane{}
	if err := c.Get(ctx, client.ObjectKey{Name: demoName, Namespace: demoNamespace}, backend); err != nil {
		t.Fatalf("get kubeadm backend: %v", err)
	}
	if backend.Spec.Version != "v1.34.0" {
		t.Fatalf("backend spec.version = %q, want %q", backend.Spec.Version, "v1.34.0")
	}
	if backend.Spec.MachineTemplate.InfrastructureRef.Kind != "DockerMachineTemplate" {
		t.Fatalf("backend machineTemplate.infrastructureRef.kind = %q, want %q", backend.Spec.MachineTemplate.InfrastructureRef.Kind, "DockerMachineTemplate")
	}
	if backend.Labels[clusterv1.ClusterNameLabel] != ownerCluster.Name {
		t.Fatalf("backend label %q = %q, want %q", clusterv1.ClusterNameLabel, backend.Labels[clusterv1.ClusterNameLabel], ownerCluster.Name)
	}

	var (
		foundControllerOwner bool
		foundClusterOwner    bool
	)
	for _, ref := range backend.OwnerReferences {
		if ref.APIVersion == "controlplane.cluster.x-k8s.io/v1alpha1" && ref.Kind == "Kany8sControlPlane" && ref.Name == cp.Name {
			foundControllerOwner = true
		}
		if ref.APIVersion == clusterv1.GroupVersion.String() && ref.Kind == "Cluster" && ref.Name == ownerCluster.Name {
			foundClusterOwner = true
		}
	}
	if !foundControllerOwner {
		t.Fatalf("kubeadm backend missing controller owner reference to Kany8sControlPlane")
	}
	if !foundClusterOwner {
		t.Fatalf("kubeadm backend missing owner reference to Cluster")
	}

	backend.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{Host: demoEndpointHost, Port: 6443}
	backend.Status.Initialization.ControlPlaneInitialized = true
	backend.Status.Conditions = []metav1.Condition{
		{
			Type:   conditionTypeReady,
			Status: metav1.ConditionTrue,
			Reason: "Ready",
		},
	}
	if err := c.Update(ctx, backend); err != nil {
		t.Fatalf("update kubeadm backend: %v", err)
	}

	res2, err := r.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("reconcile after backend ready: %v", err)
	}
	if res2.RequeueAfter != 0 {
		t.Fatalf("RequeueAfter after ready = %s, want 0", res2.RequeueAfter)
	}

	got := &controlplanev1alpha1.Kany8sControlPlane{}
	if err := c.Get(ctx, client.ObjectKey{Name: demoName, Namespace: demoNamespace}, got); err != nil {
		t.Fatalf("get control plane: %v", err)
	}
	if got.Spec.ControlPlaneEndpoint.Host != demoEndpointHost || got.Spec.ControlPlaneEndpoint.Port != 6443 {
		t.Fatalf("control plane endpoint = %s:%d, want %s:%d", got.Spec.ControlPlaneEndpoint.Host, got.Spec.ControlPlaneEndpoint.Port, demoEndpointHost, 6443)
	}
	if !got.Status.Initialization.ControlPlaneInitialized {
		t.Fatalf("control plane initialized = false, want true")
	}
	ready := meta.FindStatusCondition(got.Status.Conditions, conditionTypeReady)
	if ready == nil || ready.Status != metav1.ConditionTrue {
		t.Fatalf("control plane Ready condition = %v, want True", ready)
	}
}

func TestKany8sControlPlaneReconciler_ReconcilesExternalBackendAndInjectsSpec(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := controlplanev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add kany8s scheme: %v", err)
	}
	if err := clusterv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add cluster-api scheme: %v", err)
	}

	externalGVK := schema.GroupVersionKind{Group: "controlplane.example.com", Version: "v1alpha1", Kind: "ExampleControlPlane"}
	scheme.AddKnownTypeWithName(externalGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(externalGVK.GroupVersion().WithKind("ExampleControlPlaneList"), &unstructured.UnstructuredList{})

	cp := &controlplanev1alpha1.Kany8sControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      demoName,
			Namespace: demoNamespace,
			UID:       types.UID("00000000-0000-0000-0000-000000000000"),
		},
		Spec: controlplanev1alpha1.Kany8sControlPlaneSpec{
			Version: "v1.34.0",
			ExternalBackend: &controlplanev1alpha1.Kany8sControlPlaneExternalBackendSpec{
				APIVersion: externalGVK.GroupVersion().String(),
				Kind:       externalGVK.Kind,
				Spec:       mustJSON(t, `{"datacenter":"dc-a","version":"v0.0.0"}`),
			},
		},
	}
	ownerCluster := attachOwnerClusterToControlPlane(cp)

	var (
		watchCalled bool
		gotWatchGVK schema.GroupVersionKind
	)
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cp, ownerCluster).
		WithStatusSubresource(cp).
		Build()
	r := &Kany8sControlPlaneReconciler{
		Client: c,
		Scheme: scheme,
		InstanceWatcher: ensureWatchFunc(func(ctx context.Context, gvk schema.GroupVersionKind) error {
			watchCalled = true
			gotWatchGVK = gvk
			return nil
		}),
	}

	ctx := context.Background()
	res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: demoName, Namespace: demoNamespace}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.RequeueAfter != constants.ControlPlaneNotReadyRequeueAfter {
		t.Fatalf("RequeueAfter = %s, want %s", res.RequeueAfter, constants.ControlPlaneNotReadyRequeueAfter)
	}
	if !watchCalled {
		t.Fatalf("expected EnsureWatch to be called")
	}
	if gotWatchGVK != externalGVK {
		t.Fatalf("EnsureWatch gvk = %s, want %s", gotWatchGVK.String(), externalGVK.String())
	}

	backend := &unstructured.Unstructured{}
	backend.SetGroupVersionKind(externalGVK)
	if err := c.Get(ctx, client.ObjectKey{Name: demoName, Namespace: demoNamespace}, backend); err != nil {
		t.Fatalf("get external backend: %v", err)
	}
	spec, found, err := unstructured.NestedMap(backend.Object, "spec")
	if err != nil {
		t.Fatalf("get backend spec: %v", err)
	}
	if !found {
		t.Fatalf("backend spec not found")
	}
	if got, want := spec["datacenter"], "dc-a"; got != want {
		t.Fatalf("backend spec.datacenter = %v, want %v", got, want)
	}
	if got, want := spec["version"], "v1.34.0"; got != want {
		t.Fatalf("backend spec.version = %v, want %v", got, want)
	}
	if got, want := spec["clusterName"], ownerCluster.Name; got != want {
		t.Fatalf("backend spec.clusterName = %v, want %v", got, want)
	}
	if got, want := spec["clusterNamespace"], ownerCluster.Namespace; got != want {
		t.Fatalf("backend spec.clusterNamespace = %v, want %v", got, want)
	}
	if got, want := spec["clusterUID"], string(ownerCluster.UID); got != want {
		t.Fatalf("backend spec.clusterUID = %v, want %v", got, want)
	}
	if backend.GetLabels()[clusterv1.ClusterNameLabel] != ownerCluster.Name {
		t.Fatalf("backend label %q = %q, want %q", clusterv1.ClusterNameLabel, backend.GetLabels()[clusterv1.ClusterNameLabel], ownerCluster.Name)
	}
}

func TestKany8sControlPlaneReconciler_DeniesCrossNamespaceKubeconfigSourceForExternalBackend(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add client-go scheme: %v", err)
	}
	if err := controlplanev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add kany8s scheme: %v", err)
	}
	if err := clusterv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add cluster-api scheme: %v", err)
	}

	externalGVK := schema.GroupVersionKind{Group: "controlplane.example.com", Version: "v1alpha1", Kind: "ExampleControlPlane"}
	scheme.AddKnownTypeWithName(externalGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(externalGVK.GroupVersion().WithKind("ExampleControlPlaneList"), &unstructured.UnstructuredList{})

	cp := &controlplanev1alpha1.Kany8sControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      demoName,
			Namespace: demoNamespace,
			UID:       types.UID("00000000-0000-0000-0000-000000000000"),
		},
		Spec: controlplanev1alpha1.Kany8sControlPlaneSpec{
			Version: "v1.34.0",
			ExternalBackend: &controlplanev1alpha1.Kany8sControlPlaneExternalBackendSpec{
				APIVersion: externalGVK.GroupVersion().String(),
				Kind:       externalGVK.Kind,
			},
		},
	}
	ownerCluster := attachOwnerClusterToControlPlane(cp)

	externalObj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": externalGVK.GroupVersion().String(),
		"kind":       externalGVK.Kind,
		"metadata": map[string]any{
			"name":      demoName,
			"namespace": demoNamespace,
		},
		"spec": map[string]any{
			"version": "v1.34.0",
		},
		"status": map[string]any{
			"ready":    true,
			"endpoint": demoEndpointURL,
			"kubeconfigSecretRef": map[string]any{
				"name":      "provider-kubeconfig",
				"namespace": "other-namespace",
			},
		},
	}}
	externalObj.SetGroupVersionKind(externalGVK)

	source := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "provider-kubeconfig", Namespace: "other-namespace"},
		Data:       map[string][]byte{kubeconfig.DataKey: []byte(validKubeconfigV1)},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cp, ownerCluster, externalObj, source).
		WithStatusSubresource(cp).
		Build()
	r := &Kany8sControlPlaneReconciler{Client: c, Scheme: scheme}

	ctx := context.Background()
	res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: demoName, Namespace: demoNamespace}})
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
	cond := meta.FindStatusCondition(got.Status.Conditions, conditionTypeKubeconfigSecretReconciled)
	if cond == nil {
		t.Fatalf("expected %s condition", conditionTypeKubeconfigSecretReconciled)
	}
	if cond.Reason != reasonKubeconfigSourceSecretCrossNS {
		t.Fatalf("condition reason = %q, want %q", cond.Reason, reasonKubeconfigSourceSecretCrossNS)
	}

	target := &corev1.Secret{}
	if err := c.Get(ctx, client.ObjectKey{Name: demoName + "-kubeconfig", Namespace: demoNamespace}, target); err == nil {
		t.Fatalf("expected target kubeconfig secret to not be created")
	}
}

func mustJSON(t *testing.T, raw string) *apiextensionsv1.JSON {
	t.Helper()
	return &apiextensionsv1.JSON{Raw: []byte(raw)}
}
