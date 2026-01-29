package controlplane

import (
	"bytes"
	"context"
	"fmt"
	"strings"
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
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
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
	kcpTestClusterName       = "demo-cluster"
	kcpOwnerKindCluster      = "Cluster"
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
		Kind:       kcpOwnerKindCluster,
		Name:       kcpTestClusterName,
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
	if cond.Message == "" || !strings.Contains(cond.Message, kcpTestClusterName) {
		t.Fatalf("condition message = %q, want to contain %q", cond.Message, kcpTestClusterName)
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

	cluster := &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: kcpTestClusterName, Namespace: "default"}}

	cp := &controlplanev1alpha1.Kany8sKubeadmControlPlane{ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"}}
	cp.Spec.Version = kcpTestKubernetesVersion
	cp.Spec.MachineTemplate.InfrastructureRef = clusterv1.ContractVersionedObjectReference{APIGroup: "infrastructure.cluster.x-k8s.io", Kind: "DockerMachineTemplate", Name: "demo"}
	cp.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       kcpOwnerKindCluster,
		Name:       kcpTestClusterName,
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
	if err := bootstrapv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add KubeadmConfig scheme: %v", err)
	}
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add apiextensions scheme: %v", err)
	}

	cluster := &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: kcpTestClusterName, Namespace: "default"}}
	cluster.Spec.InfrastructureRef = clusterv1.ContractVersionedObjectReference{
		APIGroup: "infrastructure.cluster.x-k8s.io",
		Kind:     "DockerCluster",
		Name:     kcpTestClusterName,
	}

	cp := &controlplanev1alpha1.Kany8sKubeadmControlPlane{ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"}}
	cp.Spec.Version = kcpTestKubernetesVersion
	cp.Spec.MachineTemplate.InfrastructureRef = clusterv1.ContractVersionedObjectReference{APIGroup: "infrastructure.cluster.x-k8s.io", Kind: "DockerMachineTemplate", Name: "demo"}
	cp.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       kcpOwnerKindCluster,
		Name:       kcpTestClusterName,
	}}

	infraCluster := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
		"kind":       "DockerCluster",
		"metadata": map[string]any{
			"name":      kcpTestClusterName,
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

	infraMachineTemplate := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
		"kind":       "DockerMachineTemplate",
		"metadata": map[string]any{
			"name":      "demo",
			"namespace": "default",
		},
		"spec": map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"customImage": "kindest/node:v1.34.0",
				},
			},
		},
	}}

	infraMachineTemplateCRD := &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{
		Name: capicontract.CalculateCRDName("infrastructure.cluster.x-k8s.io", "DockerMachineTemplate"),
		Labels: map[string]string{
			fmt.Sprintf("%s/%s", clusterv1.GroupVersion.Group, clusterv1.GroupVersion.Version): "v1beta1",
		},
	}}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cp, cluster, infraCluster, infraCRD, infraMachineTemplate, infraMachineTemplateCRD).WithStatusSubresource(cp).Build()
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
	if err := bootstrapv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add KubeadmConfig scheme: %v", err)
	}
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add apiextensions scheme: %v", err)
	}

	cluster := &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: kcpTestClusterName, Namespace: "default", UID: types.UID("1")}}
	cluster.Spec.InfrastructureRef = clusterv1.ContractVersionedObjectReference{
		APIGroup: "infrastructure.cluster.x-k8s.io",
		Kind:     "DockerCluster",
		Name:     kcpTestClusterName,
	}

	cp := &controlplanev1alpha1.Kany8sKubeadmControlPlane{ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"}}
	cp.Spec.Version = kcpTestKubernetesVersion
	cp.Spec.MachineTemplate.InfrastructureRef = clusterv1.ContractVersionedObjectReference{APIGroup: "infrastructure.cluster.x-k8s.io", Kind: "DockerMachineTemplate", Name: "demo"}
	cp.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       kcpOwnerKindCluster,
		Name:       kcpTestClusterName,
		UID:        cluster.UID,
	}}

	infraCluster := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
		"kind":       "DockerCluster",
		"metadata": map[string]any{
			"name":      kcpTestClusterName,
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

	infraMachineTemplate := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
		"kind":       "DockerMachineTemplate",
		"metadata": map[string]any{
			"name":      "demo",
			"namespace": "default",
		},
		"spec": map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"customImage": "kindest/node:v1.34.0",
				},
			},
		},
	}}

	infraMachineTemplateCRD := &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{
		Name: capicontract.CalculateCRDName("infrastructure.cluster.x-k8s.io", "DockerMachineTemplate"),
		Labels: map[string]string{
			fmt.Sprintf("%s/%s", clusterv1.GroupVersion.Group, clusterv1.GroupVersion.Version): "v1beta1",
		},
	}}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cp, cluster, infraCluster, infraCRD, infraMachineTemplate, infraMachineTemplateCRD).WithStatusSubresource(cp).Build()
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
		key := client.ObjectKey{Namespace: "default", Name: capisecret.Name(kcpTestClusterName, purpose)}
		if err := c.Get(ctx, key, s); err != nil {
			t.Fatalf("get Secret %s: %v", key.Name, err)
		}
		if got := s.Labels[clusterv1.ClusterNameLabel]; got != kcpTestClusterName {
			t.Fatalf("Secret %s label %s = %q, want %q", key.Name, clusterv1.ClusterNameLabel, got, kcpTestClusterName)
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
		if s.OwnerReferences[0].Kind != kcpOwnerKindCluster || s.OwnerReferences[0].Name != kcpTestClusterName {
			t.Fatalf("Secret %s ownerReferences[0] = %s/%s, want %s/%s", key.Name, s.OwnerReferences[0].Kind, s.OwnerReferences[0].Name, kcpOwnerKindCluster, kcpTestClusterName)
		}
	}
}

func TestKany8sKubeadmControlPlaneReconciler_CreatesClusterKubeconfigSecretFromClusterCA(t *testing.T) {
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
	if err := bootstrapv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add KubeadmConfig scheme: %v", err)
	}
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add apiextensions scheme: %v", err)
	}

	cluster := &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: kcpTestClusterName, Namespace: "default", UID: types.UID("1")}}
	cluster.Spec.InfrastructureRef = clusterv1.ContractVersionedObjectReference{
		APIGroup: "infrastructure.cluster.x-k8s.io",
		Kind:     "DockerCluster",
		Name:     kcpTestClusterName,
	}

	cp := &controlplanev1alpha1.Kany8sKubeadmControlPlane{ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"}}
	cp.Spec.Version = kcpTestKubernetesVersion
	cp.Spec.MachineTemplate.InfrastructureRef = clusterv1.ContractVersionedObjectReference{APIGroup: "infrastructure.cluster.x-k8s.io", Kind: "DockerMachineTemplate", Name: "demo"}
	cp.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       kcpOwnerKindCluster,
		Name:       kcpTestClusterName,
		UID:        cluster.UID,
	}}

	infraCluster := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
		"kind":       "DockerCluster",
		"metadata": map[string]any{
			"name":      kcpTestClusterName,
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

	infraMachineTemplate := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
		"kind":       "DockerMachineTemplate",
		"metadata": map[string]any{
			"name":      "demo",
			"namespace": "default",
		},
		"spec": map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"customImage": "kindest/node:v1.34.0",
				},
			},
		},
	}}

	infraMachineTemplateCRD := &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{
		Name: capicontract.CalculateCRDName("infrastructure.cluster.x-k8s.io", "DockerMachineTemplate"),
		Labels: map[string]string{
			fmt.Sprintf("%s/%s", clusterv1.GroupVersion.Group, clusterv1.GroupVersion.Version): "v1beta1",
		},
	}}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cp, cluster, infraCluster, infraCRD, infraMachineTemplate, infraMachineTemplateCRD).WithStatusSubresource(cp).Build()
	r := &Kany8sKubeadmControlPlaneReconciler{Client: c, Scheme: scheme}

	ctx := context.Background()
	res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "demo", Namespace: "default"}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.RequeueAfter != 0 {
		t.Fatalf("RequeueAfter = %s, want 0", res.RequeueAfter)
	}

	ca := &corev1.Secret{}
	caKey := client.ObjectKey{Namespace: "default", Name: capisecret.Name(kcpTestClusterName, capisecret.ClusterCA)}
	if err := c.Get(ctx, caKey, ca); err != nil {
		t.Fatalf("get cluster CA Secret %s: %v", caKey.Name, err)
	}
	if len(ca.Data[capisecret.TLSCrtDataName]) == 0 {
		t.Fatalf("cluster CA Secret %s missing data[%q]", caKey.Name, capisecret.TLSCrtDataName)
	}

	s := &corev1.Secret{}
	key := client.ObjectKey{Name: "demo-cluster-kubeconfig", Namespace: "default"}
	if err := c.Get(ctx, key, s); err != nil {
		t.Fatalf("get kubeconfig Secret %s: %v", key.Name, err)
	}
	if s.Type != kubeconfig.SecretType {
		t.Fatalf("kubeconfig Secret %s type = %q, want %q", key.Name, s.Type, kubeconfig.SecretType)
	}
	if got := s.Labels[kubeconfig.ClusterNameLabelKey]; got != kcpTestClusterName {
		t.Fatalf("kubeconfig Secret %s label %s = %q, want %q", key.Name, kubeconfig.ClusterNameLabelKey, got, kcpTestClusterName)
	}
	if len(s.OwnerReferences) != 1 {
		t.Fatalf("kubeconfig Secret %s ownerReferences = %d, want %d", key.Name, len(s.OwnerReferences), 1)
	}
	if s.OwnerReferences[0].Kind != kcpOwnerKindCluster || s.OwnerReferences[0].Name != kcpTestClusterName {
		t.Fatalf("kubeconfig Secret %s ownerReferences[0] = %s/%s, want %s/%s", key.Name, s.OwnerReferences[0].Kind, s.OwnerReferences[0].Name, kcpOwnerKindCluster, kcpTestClusterName)
	}
	if len(s.Data[kubeconfig.DataKey]) == 0 {
		t.Fatalf("kubeconfig Secret %s missing data[%q]", key.Name, kubeconfig.DataKey)
	}

	clientConfig, err := clientcmd.NewClientConfigFromBytes(s.Data[kubeconfig.DataKey])
	if err != nil {
		t.Fatalf("parse kubeconfig: %v", err)
	}
	restCfg, err := clientConfig.ClientConfig()
	if err != nil {
		t.Fatalf("build rest config: %v", err)
	}
	if restCfg.Host != "https://127.0.0.1:6443" {
		t.Fatalf("kubeconfig server = %q, want %q", restCfg.Host, "https://127.0.0.1:6443")
	}
	if !bytes.Equal(restCfg.CAData, ca.Data[capisecret.TLSCrtDataName]) {
		t.Fatalf("kubeconfig CAData does not match cluster CA Secret")
	}
}

func TestKany8sKubeadmControlPlaneReconciler_CreatesInfraMachineFromMachineTemplate(t *testing.T) {
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
	if err := bootstrapv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add KubeadmConfig scheme: %v", err)
	}
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add apiextensions scheme: %v", err)
	}

	cluster := &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: kcpTestClusterName, Namespace: "default", UID: types.UID("1")}}
	cluster.Spec.InfrastructureRef = clusterv1.ContractVersionedObjectReference{
		APIGroup: "infrastructure.cluster.x-k8s.io",
		Kind:     "DockerCluster",
		Name:     kcpTestClusterName,
	}

	cp := &controlplanev1alpha1.Kany8sKubeadmControlPlane{ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"}}
	cp.Spec.Version = kcpTestKubernetesVersion
	cp.Spec.MachineTemplate.InfrastructureRef = clusterv1.ContractVersionedObjectReference{APIGroup: "infrastructure.cluster.x-k8s.io", Kind: "DockerMachineTemplate", Name: "demo"}
	cp.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       kcpOwnerKindCluster,
		Name:       kcpTestClusterName,
		UID:        cluster.UID,
	}}

	infraCluster := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
		"kind":       "DockerCluster",
		"metadata": map[string]any{
			"name":      kcpTestClusterName,
			"namespace": "default",
		},
		"spec": map[string]any{
			"controlPlaneEndpoint": map[string]any{
				"host": "127.0.0.1",
				"port": int64(6443),
			},
		},
	}}

	infraClusterCRD := &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{
		Name: capicontract.CalculateCRDName("infrastructure.cluster.x-k8s.io", "DockerCluster"),
		Labels: map[string]string{
			fmt.Sprintf("%s/%s", clusterv1.GroupVersion.Group, clusterv1.GroupVersion.Version): "v1beta1",
		},
	}}

	infraMachineTemplate := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
		"kind":       "DockerMachineTemplate",
		"metadata": map[string]any{
			"name":      "demo",
			"namespace": "default",
		},
		"spec": map[string]any{
			"template": map[string]any{
				"metadata": map[string]any{
					"labels": map[string]any{
						"template-label": "true",
					},
				},
				"spec": map[string]any{
					"customImage": "kindest/node:v1.34.0",
				},
			},
		},
	}}

	infraMachineTemplateCRD := &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{
		Name: capicontract.CalculateCRDName("infrastructure.cluster.x-k8s.io", "DockerMachineTemplate"),
		Labels: map[string]string{
			fmt.Sprintf("%s/%s", clusterv1.GroupVersion.Group, clusterv1.GroupVersion.Version): "v1beta1",
		},
	}}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cp, cluster, infraCluster, infraClusterCRD, infraMachineTemplate, infraMachineTemplateCRD).WithStatusSubresource(cp).Build()
	r := &Kany8sKubeadmControlPlaneReconciler{Client: c, Scheme: scheme}

	ctx := context.Background()
	res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "demo", Namespace: "default"}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.RequeueAfter != 0 {
		t.Fatalf("RequeueAfter = %s, want 0", res.RequeueAfter)
	}

	infraMachine := &unstructured.Unstructured{}
	infraMachine.SetAPIVersion("infrastructure.cluster.x-k8s.io/v1beta1")
	infraMachine.SetKind("DockerMachine")
	key := client.ObjectKey{Name: "demo-cluster-control-plane-0", Namespace: "default"}
	if err := c.Get(ctx, key, infraMachine); err != nil {
		t.Fatalf("get infraMachine %s: %v", key.Name, err)
	}
	if got := infraMachine.GetLabels()[clusterv1.ClusterNameLabel]; got != kcpTestClusterName {
		t.Fatalf("infraMachine %s label %s = %q, want %q", key.Name, clusterv1.ClusterNameLabel, got, kcpTestClusterName)
	}
	if len(infraMachine.GetOwnerReferences()) != 1 {
		t.Fatalf("infraMachine %s ownerReferences = %d, want %d", key.Name, len(infraMachine.GetOwnerReferences()), 1)
	}
	ownerRef := infraMachine.GetOwnerReferences()[0]
	if ownerRef.Kind != kcpOwnerKindCluster || ownerRef.Name != kcpTestClusterName {
		t.Fatalf("infraMachine %s ownerReferences[0] = %s/%s, want %s/%s", key.Name, ownerRef.Kind, ownerRef.Name, kcpOwnerKindCluster, kcpTestClusterName)
	}
}

func TestKany8sKubeadmControlPlaneReconciler_CreatesKubeadmConfigForInitialControlPlane(t *testing.T) {
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
	if err := bootstrapv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add KubeadmConfig scheme: %v", err)
	}
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add apiextensions scheme: %v", err)
	}

	cluster := &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: kcpTestClusterName, Namespace: "default", UID: types.UID("1")}}
	cluster.Spec.InfrastructureRef = clusterv1.ContractVersionedObjectReference{
		APIGroup: "infrastructure.cluster.x-k8s.io",
		Kind:     "DockerCluster",
		Name:     kcpTestClusterName,
	}

	cp := &controlplanev1alpha1.Kany8sKubeadmControlPlane{ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"}}
	cp.Spec.Version = kcpTestKubernetesVersion
	cp.Spec.MachineTemplate.InfrastructureRef = clusterv1.ContractVersionedObjectReference{APIGroup: "infrastructure.cluster.x-k8s.io", Kind: "DockerMachineTemplate", Name: "demo"}
	cp.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       kcpOwnerKindCluster,
		Name:       kcpTestClusterName,
		UID:        cluster.UID,
	}}

	infraCluster := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
		"kind":       "DockerCluster",
		"metadata": map[string]any{
			"name":      kcpTestClusterName,
			"namespace": "default",
		},
		"spec": map[string]any{
			"controlPlaneEndpoint": map[string]any{
				"host": "127.0.0.1",
				"port": int64(6443),
			},
		},
	}}

	infraClusterCRD := &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{
		Name: capicontract.CalculateCRDName("infrastructure.cluster.x-k8s.io", "DockerCluster"),
		Labels: map[string]string{
			fmt.Sprintf("%s/%s", clusterv1.GroupVersion.Group, clusterv1.GroupVersion.Version): "v1beta1",
		},
	}}

	infraMachineTemplate := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
		"kind":       "DockerMachineTemplate",
		"metadata": map[string]any{
			"name":      "demo",
			"namespace": "default",
		},
		"spec": map[string]any{
			"template": map[string]any{
				"metadata": map[string]any{
					"labels": map[string]any{
						"template-label": "true",
					},
				},
				"spec": map[string]any{
					"customImage": "kindest/node:v1.34.0",
				},
			},
		},
	}}

	infraMachineTemplateCRD := &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{
		Name: capicontract.CalculateCRDName("infrastructure.cluster.x-k8s.io", "DockerMachineTemplate"),
		Labels: map[string]string{
			fmt.Sprintf("%s/%s", clusterv1.GroupVersion.Group, clusterv1.GroupVersion.Version): "v1beta1",
		},
	}}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cp, cluster, infraCluster, infraClusterCRD, infraMachineTemplate, infraMachineTemplateCRD).
		WithStatusSubresource(cp).
		Build()
	r := &Kany8sKubeadmControlPlaneReconciler{Client: c, Scheme: scheme}

	ctx := context.Background()
	res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "demo", Namespace: "default"}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.RequeueAfter != 0 {
		t.Fatalf("RequeueAfter = %s, want 0", res.RequeueAfter)
	}

	kc := &bootstrapv1.KubeadmConfig{}
	key := client.ObjectKey{Name: "demo-cluster-control-plane-0", Namespace: "default"}
	if err := c.Get(ctx, key, kc); err != nil {
		t.Fatalf("get KubeadmConfig %s: %v", key.Name, err)
	}
	if got := kc.Spec.ClusterConfiguration.ControlPlaneEndpoint; got != "127.0.0.1:6443" {
		t.Fatalf("KubeadmConfig.spec.clusterConfiguration.controlPlaneEndpoint = %q, want %q", got, "127.0.0.1:6443")
	}
	if got := kc.Spec.InitConfiguration.LocalAPIEndpoint.BindPort; got != 6443 {
		t.Fatalf("KubeadmConfig.spec.initConfiguration.localAPIEndpoint.bindPort = %d, want %d", got, 6443)
	}

	foundCA := false
	for _, f := range kc.Spec.Files {
		if f.Path == "/etc/kubernetes/pki/ca.crt" && len(f.Content) > 0 {
			foundCA = true
			break
		}
	}
	if !foundCA {
		t.Fatalf("expected KubeadmConfig.spec.files to include %q with non-empty content", "/etc/kubernetes/pki/ca.crt")
	}
}
