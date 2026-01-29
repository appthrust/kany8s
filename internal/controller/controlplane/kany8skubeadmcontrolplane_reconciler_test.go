package controlplane

import (
	"context"
	"fmt"
	"strings"
	"testing"

	controlplanev1alpha1 "github.com/reoring/kany8s/api/v1alpha1"
	"github.com/reoring/kany8s/internal/constants"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	capicontract "sigs.k8s.io/cluster-api/util/contract"
	capisecret "sigs.k8s.io/cluster-api/util/secret"
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
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
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
	if cond.Status != metav1.ConditionTrue {
		t.Fatalf("condition status = %q, want %q", cond.Status, metav1.ConditionTrue)
	}
	if cond.Reason != kcpReasonOwnerClusterResolved {
		t.Fatalf("condition reason = %q, want %q", cond.Reason, kcpReasonOwnerClusterResolved)
	}
}

func TestKany8sKubeadmControlPlaneReconciler_SetsControlPlaneEndpointFromInfrastructureCluster(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := controlplanev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Kany8sKubeadmControlPlane scheme: %v", err)
	}
	if err := clusterv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Cluster scheme: %v", err)
	}
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add apiextensions scheme: %v", err)
	}

	cluster := &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "demo-cluster", Namespace: "default"}}
	cluster.Spec.InfrastructureRef = clusterv1.ContractVersionedObjectReference{
		APIGroup: "infrastructure.cluster.x-k8s.io",
		Kind:     "DockerCluster",
		Name:     "demo-cluster",
	}

	cp := &controlplanev1alpha1.Kany8sKubeadmControlPlane{ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"}}
	cp.Spec.Version = kcpTestKubernetesVersion
	cp.Spec.MachineTemplate.InfrastructureRef = clusterv1.ContractVersionedObjectReference{APIGroup: "infrastructure.cluster.x-k8s.io", Kind: "DockerMachineTemplate", Name: "demo"}
	cp.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       "demo-cluster",
	}}

	infraCluster := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
		"kind":       "DockerCluster",
		"metadata": map[string]any{
			"name":      "demo-cluster",
			"namespace": "default",
		},
		"spec": map[string]any{
			"controlPlaneEndpoint": map[string]any{
				"host": "127.0.0.1",
				"port": int64(6443),
			},
		},
	}}

	infraCRD := &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{
		Name: capicontract.CalculateCRDName("infrastructure.cluster.x-k8s.io", "DockerCluster"),
		Labels: map[string]string{
			fmt.Sprintf("%s/%s", clusterv1.GroupVersion.Group, clusterv1.GroupVersion.Version): "v1beta1",
		},
	}}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cp, cluster, infraCluster, infraCRD).WithStatusSubresource(cp).Build()
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

	if got.Spec.ControlPlaneEndpoint.Host != "127.0.0.1" {
		t.Fatalf("spec.controlPlaneEndpoint.host = %q, want %q", got.Spec.ControlPlaneEndpoint.Host, "127.0.0.1")
	}
	if got.Spec.ControlPlaneEndpoint.Port != 6443 {
		t.Fatalf("spec.controlPlaneEndpoint.port = %d, want %d", got.Spec.ControlPlaneEndpoint.Port, 6443)
	}
}

func TestKany8sKubeadmControlPlaneReconciler_GeneratesClusterCertificatesSecrets(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := controlplanev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Kany8sKubeadmControlPlane scheme: %v", err)
	}
	if err := clusterv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Cluster scheme: %v", err)
	}
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add apiextensions scheme: %v", err)
	}

	cluster := &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "demo-cluster", Namespace: "default", UID: types.UID("1")}}
	cluster.Spec.InfrastructureRef = clusterv1.ContractVersionedObjectReference{
		APIGroup: "infrastructure.cluster.x-k8s.io",
		Kind:     "DockerCluster",
		Name:     "demo-cluster",
	}

	cp := &controlplanev1alpha1.Kany8sKubeadmControlPlane{ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"}}
	cp.Spec.Version = kcpTestKubernetesVersion
	cp.Spec.MachineTemplate.InfrastructureRef = clusterv1.ContractVersionedObjectReference{APIGroup: "infrastructure.cluster.x-k8s.io", Kind: "DockerMachineTemplate", Name: "demo"}
	cp.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       "demo-cluster",
		UID:        cluster.UID,
	}}

	infraCluster := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
		"kind":       "DockerCluster",
		"metadata": map[string]any{
			"name":      "demo-cluster",
			"namespace": "default",
		},
		"spec": map[string]any{
			"controlPlaneEndpoint": map[string]any{
				"host": "127.0.0.1",
				"port": int64(6443),
			},
		},
	}}

	infraCRD := &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{
		Name: capicontract.CalculateCRDName("infrastructure.cluster.x-k8s.io", "DockerCluster"),
		Labels: map[string]string{
			fmt.Sprintf("%s/%s", clusterv1.GroupVersion.Group, clusterv1.GroupVersion.Version): "v1beta1",
		},
	}}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cp, cluster, infraCluster, infraCRD).WithStatusSubresource(cp).Build()
	r := &Kany8sKubeadmControlPlaneReconciler{Client: c, Scheme: scheme}

	ctx := context.Background()
	res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "demo", Namespace: "default"}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.RequeueAfter != 0 {
		t.Fatalf("RequeueAfter = %s, want 0", res.RequeueAfter)
	}

	for _, purpose := range []capisecret.Purpose{capisecret.ClusterCA, capisecret.EtcdCA, capisecret.FrontProxyCA, capisecret.ServiceAccount} {
		s := &corev1.Secret{}
		key := client.ObjectKey{Namespace: "default", Name: capisecret.Name("demo-cluster", purpose)}
		if err := c.Get(ctx, key, s); err != nil {
			t.Fatalf("get Secret %s: %v", key.Name, err)
		}
		if got := s.Labels[clusterv1.ClusterNameLabel]; got != "demo-cluster" {
			t.Fatalf("Secret %s label %s = %q, want %q", key.Name, clusterv1.ClusterNameLabel, got, "demo-cluster")
		}
		if s.Type != clusterv1.ClusterSecretType {
			t.Fatalf("Secret %s type = %q, want %q", key.Name, s.Type, clusterv1.ClusterSecretType)
		}
		if len(s.Data[capisecret.TLSCrtDataName]) == 0 {
			t.Fatalf("Secret %s missing data[%q]", key.Name, capisecret.TLSCrtDataName)
		}
		if len(s.Data[capisecret.TLSKeyDataName]) == 0 {
			t.Fatalf("Secret %s missing data[%q]", key.Name, capisecret.TLSKeyDataName)
		}
		if len(s.OwnerReferences) != 1 {
			t.Fatalf("Secret %s ownerReferences = %d, want %d", key.Name, len(s.OwnerReferences), 1)
		}
		if s.OwnerReferences[0].Kind != "Cluster" || s.OwnerReferences[0].Name != "demo-cluster" {
			t.Fatalf("Secret %s ownerReferences[0] = %s/%s, want Cluster/demo-cluster", key.Name, s.OwnerReferences[0].Kind, s.OwnerReferences[0].Name)
		}
	}
}
