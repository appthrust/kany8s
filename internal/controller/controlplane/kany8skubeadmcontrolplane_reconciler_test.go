package controlplane

import (
	"context"
	"strings"
	"testing"

	controlplanev1alpha1 "github.com/reoring/kany8s/api/v1alpha1"
	"github.com/reoring/kany8s/internal/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	kcpConditionTypeOwnerClusterResolved = "OwnerClusterResolved"

	kcpReasonOwnerClusterNotSet   = "OwnerClusterNotSet"
	kcpReasonOwnerClusterNotFound = "OwnerClusterNotFound"
	kcpReasonOwnerClusterResolved = "OwnerClusterResolved"

	kcpTestKubernetesVersion = "1.34"
)

func TestKany8sKubeadmControlPlaneReconciler_RequeuesWhenOwnerClusterNotSet(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := controlplanev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Kany8sKubeadmControlPlane scheme: %v", err)
	}

	cp := &controlplanev1alpha1.Kany8sKubeadmControlPlane{ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"}}
	cp.Spec.Version = kcpTestKubernetesVersion
	cp.Spec.MachineTemplate.InfrastructureRef = clusterv1.ContractVersionedObjectReference{APIGroup: "infrastructure.cluster.x-k8s.io", Kind: "DockerMachineTemplate", Name: "demo"}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cp).WithStatusSubresource(cp).Build()
	r := &Kany8sKubeadmControlPlaneReconciler{Client: c, Scheme: scheme}

	ctx := context.Background()
	res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "demo", Namespace: "default"}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.RequeueAfter != constants.ControlPlaneNotReadyRequeueAfter {
		t.Fatalf("RequeueAfter = %s, want %s", res.RequeueAfter, constants.ControlPlaneNotReadyRequeueAfter)
	}

	got := &controlplanev1alpha1.Kany8sKubeadmControlPlane{}
	if err := c.Get(ctx, client.ObjectKey{Name: "demo", Namespace: "default"}, got); err != nil {
		t.Fatalf("get control plane: %v", err)
	}

	cond := meta.FindStatusCondition(got.Status.Conditions, kcpConditionTypeOwnerClusterResolved)
	if cond == nil {
		t.Fatalf("expected %q condition", kcpConditionTypeOwnerClusterResolved)
	}
	if cond.Status != metav1.ConditionFalse {
		t.Fatalf("condition status = %q, want %q", cond.Status, metav1.ConditionFalse)
	}
	if cond.Reason != kcpReasonOwnerClusterNotSet {
		t.Fatalf("condition reason = %q, want %q", cond.Reason, kcpReasonOwnerClusterNotSet)
	}
	if cond.Message == "" || !strings.Contains(cond.Message, "owner Cluster") {
		t.Fatalf("condition message = %q, want to contain %q", cond.Message, "owner Cluster")
	}
}

func TestKany8sKubeadmControlPlaneReconciler_RequeuesWhenOwnerClusterNotFound(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := controlplanev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Kany8sKubeadmControlPlane scheme: %v", err)
	}
	if err := clusterv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Cluster scheme: %v", err)
	}

	cp := &controlplanev1alpha1.Kany8sKubeadmControlPlane{ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"}}
	cp.Spec.Version = kcpTestKubernetesVersion
	cp.Spec.MachineTemplate.InfrastructureRef = clusterv1.ContractVersionedObjectReference{APIGroup: "infrastructure.cluster.x-k8s.io", Kind: "DockerMachineTemplate", Name: "demo"}
	cp.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       "demo-cluster",
	}}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cp).WithStatusSubresource(cp).Build()
	r := &Kany8sKubeadmControlPlaneReconciler{Client: c, Scheme: scheme}

	ctx := context.Background()
	res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "demo", Namespace: "default"}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.RequeueAfter != constants.ControlPlaneNotReadyRequeueAfter {
		t.Fatalf("RequeueAfter = %s, want %s", res.RequeueAfter, constants.ControlPlaneNotReadyRequeueAfter)
	}

	got := &controlplanev1alpha1.Kany8sKubeadmControlPlane{}
	if err := c.Get(ctx, client.ObjectKey{Name: "demo", Namespace: "default"}, got); err != nil {
		t.Fatalf("get control plane: %v", err)
	}

	cond := meta.FindStatusCondition(got.Status.Conditions, kcpConditionTypeOwnerClusterResolved)
	if cond == nil {
		t.Fatalf("expected %q condition", kcpConditionTypeOwnerClusterResolved)
	}
	if cond.Status != metav1.ConditionFalse {
		t.Fatalf("condition status = %q, want %q", cond.Status, metav1.ConditionFalse)
	}
	if cond.Reason != kcpReasonOwnerClusterNotFound {
		t.Fatalf("condition reason = %q, want %q", cond.Reason, kcpReasonOwnerClusterNotFound)
	}
	if cond.Message == "" || !strings.Contains(cond.Message, "demo-cluster") {
		t.Fatalf("condition message = %q, want to contain %q", cond.Message, "demo-cluster")
	}
}

func TestKany8sKubeadmControlPlaneReconciler_SetsOwnerClusterResolvedConditionTrue(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := controlplanev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Kany8sKubeadmControlPlane scheme: %v", err)
	}
	if err := clusterv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Cluster scheme: %v", err)
	}

	cluster := &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "demo-cluster", Namespace: "default"}}

	cp := &controlplanev1alpha1.Kany8sKubeadmControlPlane{ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"}}
	cp.Spec.Version = kcpTestKubernetesVersion
	cp.Spec.MachineTemplate.InfrastructureRef = clusterv1.ContractVersionedObjectReference{APIGroup: "infrastructure.cluster.x-k8s.io", Kind: "DockerMachineTemplate", Name: "demo"}
	cp.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       "demo-cluster",
	}}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cp, cluster).WithStatusSubresource(cp).Build()
	r := &Kany8sKubeadmControlPlaneReconciler{Client: c, Scheme: scheme}

	ctx := context.Background()
	res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "demo", Namespace: "default"}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.RequeueAfter != 0 {
		t.Fatalf("RequeueAfter = %s, want 0", res.RequeueAfter)
	}

	got := &controlplanev1alpha1.Kany8sKubeadmControlPlane{}
	if err := c.Get(ctx, client.ObjectKey{Name: "demo", Namespace: "default"}, got); err != nil {
		t.Fatalf("get control plane: %v", err)
	}

	cond := meta.FindStatusCondition(got.Status.Conditions, kcpConditionTypeOwnerClusterResolved)
	if cond == nil {
		t.Fatalf("expected %q condition", kcpConditionTypeOwnerClusterResolved)
	}
	if cond.Status != metav1.ConditionTrue {
		t.Fatalf("condition status = %q, want %q", cond.Status, metav1.ConditionTrue)
	}
	if cond.Reason != kcpReasonOwnerClusterResolved {
		t.Fatalf("condition reason = %q, want %q", cond.Reason, kcpReasonOwnerClusterResolved)
	}
}
