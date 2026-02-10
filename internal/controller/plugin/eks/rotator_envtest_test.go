package eks

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/reoring/kany8s/internal/kubeconfig"
	coreeks "github.com/reoring/kany8s/internal/plugin/eks"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestEKSKubeconfigRotatorReconciler_Envtest_ControlPlaneRefOwnershipConflictAndRequeuePolicy(t *testing.T) {
	t.Parallel()

	h := startEKSEnvtestHarness(
		t,
		stubCAPIClusterCRD(),
		stubNamespacedCRD("eks.services.k8s.aws", "v1alpha1", "Cluster", "clusters"),
	)

	now := time.Date(2026, 2, 9, 10, 0, 0, 0, time.UTC)
	expiration := now.Add(14 * time.Minute)
	tokenGen := &capturingTokenGenerator{
		token:      "k8s-aws-v1.envtest",
		expiration: expiration,
	}

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
			Annotations: map[string]string{
				coreeks.EnableAnnotationKey: coreeks.EnableAnnotationValue,
			},
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: kany8sControlPlaneAPIGroup,
				Kind:     kany8sControlPlaneKind,
				Name:     "demo-cp",
			},
		},
	}
	if err := h.client.Create(context.Background(), cluster); err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	caData := base64.StdEncoding.EncodeToString([]byte("ca-cert"))
	ack := ackClusterObj("default", "demo-cp", "https://demo.example", caData, "ap-northeast-1")
	if err := h.client.Create(context.Background(), ack); err != nil {
		t.Fatalf("create ACK cluster: %v", err)
	}

	probeName, err := kubeconfig.SecretName("demo")
	if err != nil {
		t.Fatalf("SecretName() error = %v", err)
	}
	unmanagedProbe := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: probeName, Namespace: "default"},
		Type:       kubeconfig.SecretType,
		Data:       map[string][]byte{kubeconfig.DataKey: []byte("unmanaged")},
	}
	if err := h.client.Create(context.Background(), unmanagedProbe); err != nil {
		t.Fatalf("create unmanaged probe secret: %v", err)
	}

	r := &EKSKubeconfigRotatorReconciler{
		Client:         h.client,
		Scheme:         h.scheme,
		TokenGenerator: tokenGen,
		Policy: coreeks.RequeuePolicy{
			RefreshBefore:  5 * time.Minute,
			MaxRefresh:     7 * time.Minute,
			FailureBackoff: 20 * time.Second,
		},
		Now: func() time.Time { return now },
	}

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Namespace: "default", Name: "demo"}})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if got, want := res.RequeueAfter, 7*time.Minute; got != want {
		t.Fatalf("RequeueAfter = %s, want %s", got, want)
	}
	if got, want := tokenGen.gotClusterName, "demo-cp"; got != want {
		t.Fatalf("token clusterName = %q, want %q", got, want)
	}
	if got, want := tokenGen.gotRegion, "ap-northeast-1"; got != want {
		t.Fatalf("token region = %q, want %q", got, want)
	}

	gotProbe := &corev1.Secret{}
	if err := h.client.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: probeName}, gotProbe); err != nil {
		t.Fatalf("get probe secret: %v", err)
	}
	if got, want := string(gotProbe.Data[kubeconfig.DataKey]), "unmanaged"; got != want {
		t.Fatalf("probe secret data = %q, want %q", got, want)
	}
}
