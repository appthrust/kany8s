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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/clientcmd"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type fakeTokenGenerator struct {
	token      string
	expiration time.Time
	err        error
}

func (f fakeTokenGenerator) Generate(_ context.Context, _, _ string) (string, time.Time, error) {
	return f.token, f.expiration, f.err
}

type capturingTokenGenerator struct {
	token      string
	expiration time.Time
	err        error

	gotRegion      string
	gotClusterName string
	callCount      int
}

func (c *capturingTokenGenerator) Generate(_ context.Context, region, clusterName string) (string, time.Time, error) {
	c.callCount++
	c.gotRegion = region
	c.gotClusterName = clusterName
	return c.token, c.expiration, c.err
}

func TestEKSKubeconfigRotatorReconciler_ReconcileCreatesProbeAndExecSecrets(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))

	now := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)
	expiration := now.Add(14 * time.Minute)
	caData := base64.StdEncoding.EncodeToString([]byte("ca-cert"))

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
			UID:       "cluster-uid",
			Annotations: map[string]string{
				coreeks.EnableAnnotationKey:         coreeks.EnableAnnotationValue,
				coreeks.EKSClusterNameAnnotationKey: "eks-demo",
				coreeks.RegionAnnotationKey:         "ap-northeast-1",
			},
		},
	}
	ack := ackClusterObj("default", "eks-demo", "https://demo.example", caData, "ap-northeast-1")

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, ack).Build()

	r := &EKSKubeconfigRotatorReconciler{
		Client: c,
		Scheme: scheme,
		TokenGenerator: fakeTokenGenerator{
			token:      "k8s-aws-v1.test",
			expiration: expiration,
		},
		Policy: coreeks.RequeuePolicy{
			RefreshBefore:  5 * time.Minute,
			MaxRefresh:     10 * time.Minute,
			FailureBackoff: 30 * time.Second,
		},
		Now: func() time.Time { return now },
	}

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: "demo", Namespace: "default"}})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if got, want := res.RequeueAfter, 9*time.Minute; got != want {
		t.Fatalf("RequeueAfter = %s, want %s", got, want)
	}

	probeName, err := kubeconfig.SecretName("demo")
	if err != nil {
		t.Fatalf("SecretName() error = %v", err)
	}
	probe := &corev1.Secret{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: probeName}, probe); err != nil {
		t.Fatalf("get probe secret: %v", err)
	}
	if got, want := probe.Type, kubeconfig.SecretType; got != want {
		t.Fatalf("probe secret type = %q, want %q", got, want)
	}
	if got, want := probe.Annotations[coreeks.ManagedByAnnotationKey], coreeks.ManagedByAnnotationValue; got != want {
		t.Fatalf("probe managed annotation = %q, want %q", got, want)
	}
	if got, want := probe.Annotations[coreeks.TokenExpirationAnnotationKey], expiration.UTC().Format(time.RFC3339); got != want {
		t.Fatalf("probe expiration annotation = %q, want %q", got, want)
	}
	probeCfg, err := clientcmd.Load(probe.Data[kubeconfig.DataKey])
	if err != nil {
		t.Fatalf("load probe kubeconfig: %v", err)
	}
	if got, want := probeCfg.AuthInfos["aws"].Token, "k8s-aws-v1.test"; got != want {
		t.Fatalf("probe token = %q, want %q", got, want)
	}

	execSecret := &corev1.Secret{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: probeName + "-exec"}, execSecret); err != nil {
		t.Fatalf("get exec secret: %v", err)
	}
	execCfg, err := clientcmd.Load(execSecret.Data[kubeconfig.DataKey])
	if err != nil {
		t.Fatalf("load exec kubeconfig: %v", err)
	}
	if got, want := execCfg.AuthInfos["aws"].Exec.Command, "aws"; got != want {
		t.Fatalf("exec command = %q, want %q", got, want)
	}
}

func TestEKSKubeconfigRotatorReconciler_ReconcileRequeuesWhenACKStatusIsIncomplete(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
			Annotations: map[string]string{
				coreeks.EnableAnnotationKey: coreeks.EnableAnnotationValue,
			},
		},
	}
	ack := ackClusterObj("default", "demo", "", "", "ap-northeast-1")

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, ack).Build()
	r := &EKSKubeconfigRotatorReconciler{
		Client: c,
		Scheme: scheme,
		TokenGenerator: fakeTokenGenerator{
			token:      "k8s-aws-v1.test",
			expiration: time.Now().Add(10 * time.Minute),
		},
		Policy: coreeks.RequeuePolicy{FailureBackoff: 45 * time.Second},
		Now:    time.Now,
	}

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: "demo", Namespace: "default"}})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if got, want := res.RequeueAfter, 45*time.Second; got != want {
		t.Fatalf("RequeueAfter = %s, want %s", got, want)
	}

	probeName, _ := kubeconfig.SecretName("demo")
	probe := &corev1.Secret{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: probeName}, probe); err == nil {
		t.Fatalf("probe secret should not exist when ACK status is incomplete")
	}
}

func TestEKSKubeconfigRotatorReconciler_DoesNotOverwriteUnmanagedProbeSecret(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
			Annotations: map[string]string{
				coreeks.EnableAnnotationKey: coreeks.EnableAnnotationValue,
			},
		},
	}
	caData := base64.StdEncoding.EncodeToString([]byte("ca-cert"))
	ack := ackClusterObj("default", "demo", "https://demo.example", caData, "ap-northeast-1")
	probeName, _ := kubeconfig.SecretName("demo")
	existingProbe := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: probeName, Namespace: "default"},
		Type:       kubeconfig.SecretType,
		Data:       map[string][]byte{kubeconfig.DataKey: []byte("unmanaged")},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, ack, existingProbe).Build()
	r := &EKSKubeconfigRotatorReconciler{
		Client: c,
		Scheme: scheme,
		TokenGenerator: fakeTokenGenerator{
			token:      "k8s-aws-v1.test",
			expiration: time.Now().Add(12 * time.Minute),
		},
		Policy: coreeks.RequeuePolicy{MaxRefresh: 7 * time.Minute},
		Now:    time.Now,
	}

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: "demo", Namespace: "default"}})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if got, want := res.RequeueAfter, 7*time.Minute; got != want {
		t.Fatalf("RequeueAfter = %s, want %s", got, want)
	}

	gotProbe := &corev1.Secret{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: probeName}, gotProbe); err != nil {
		t.Fatalf("get probe secret: %v", err)
	}
	if got, want := string(gotProbe.Data[kubeconfig.DataKey]), "unmanaged"; got != want {
		t.Fatalf("probe secret data was overwritten: got %q, want %q", got, want)
	}
}

func TestEKSKubeconfigRotatorReconciler_TakesOverUnmanagedSecretsWhenExplicitlyAllowed(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))

	now := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)
	expiration := now.Add(14 * time.Minute)
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
			Annotations: map[string]string{
				coreeks.EnableAnnotationKey:                 coreeks.EnableAnnotationValue,
				coreeks.AllowUnmanagedTakeoverAnnotationKey: coreeks.AllowUnmanagedTakeoverAnnotationValue,
			},
		},
	}
	caData := base64.StdEncoding.EncodeToString([]byte("ca-cert"))
	ack := ackClusterObj("default", "demo", "https://demo.example", caData, "ap-northeast-1")
	probeName, _ := kubeconfig.SecretName("demo")
	execName := probeName + "-exec"
	unmanagedProbe := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: probeName, Namespace: "default"},
		Type:       kubeconfig.SecretType,
		Data:       map[string][]byte{kubeconfig.DataKey: []byte("unmanaged-probe")},
	}
	unmanagedExec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: execName, Namespace: "default"},
		Type:       kubeconfig.SecretType,
		Data:       map[string][]byte{kubeconfig.DataKey: []byte("unmanaged-exec")},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, ack, unmanagedProbe, unmanagedExec).Build()
	r := &EKSKubeconfigRotatorReconciler{
		Client: c,
		Scheme: scheme,
		TokenGenerator: fakeTokenGenerator{
			token:      "k8s-aws-v1.takeover",
			expiration: expiration,
		},
		Policy: coreeks.RequeuePolicy{
			RefreshBefore:  5 * time.Minute,
			MaxRefresh:     10 * time.Minute,
			FailureBackoff: 30 * time.Second,
		},
		Now: func() time.Time { return now },
	}

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: "demo", Namespace: "default"}})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if got, want := res.RequeueAfter, 9*time.Minute; got != want {
		t.Fatalf("RequeueAfter = %s, want %s", got, want)
	}

	gotProbe := &corev1.Secret{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: probeName}, gotProbe); err != nil {
		t.Fatalf("get probe secret: %v", err)
	}
	if got, want := gotProbe.Annotations[coreeks.ManagedByAnnotationKey], coreeks.ManagedByAnnotationValue; got != want {
		t.Fatalf("probe managed annotation = %q, want %q", got, want)
	}
	if got := string(gotProbe.Data[kubeconfig.DataKey]); got == "unmanaged-probe" {
		t.Fatalf("probe secret data was not taken over")
	}

	gotExec := &corev1.Secret{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: execName}, gotExec); err != nil {
		t.Fatalf("get exec secret: %v", err)
	}
	if got, want := gotExec.Annotations[coreeks.ManagedByAnnotationKey], coreeks.ManagedByAnnotationValue; got != want {
		t.Fatalf("exec managed annotation = %q, want %q", got, want)
	}
	if got := string(gotExec.Data[kubeconfig.DataKey]); got == "unmanaged-exec" {
		t.Fatalf("exec secret data was not taken over")
	}
}

func TestResolveClusterNames_UsesKany8sControlPlaneRefByDefault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		cluster      *clusterv1.Cluster
		wantCAPIName string
		wantEKSName  string
		wantACKName  string
	}{
		{
			name: "defaults to controlPlaneRef.name for kany8s control plane",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "demo",
					Namespace: "default",
				},
				Spec: clusterv1.ClusterSpec{
					ControlPlaneRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: kany8sControlPlaneAPIGroup,
						Kind:     kany8sControlPlaneKind,
						Name:     "demo-cp",
					},
				},
			},
			wantCAPIName: "demo",
			wantEKSName:  "demo-cp",
			wantACKName:  "demo-cp",
		},
		{
			name: "falls back to cluster.name for non-kany8s control plane kind",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "demo",
					Namespace: "default",
				},
				Spec: clusterv1.ClusterSpec{
					ControlPlaneRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: kany8sControlPlaneAPIGroup,
						Kind:     "OtherControlPlane",
						Name:     "other-cp",
					},
				},
			},
			wantCAPIName: "demo",
			wantEKSName:  "demo",
			wantACKName:  "demo",
		},
		{
			name: "annotation override still wins",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "demo",
					Namespace: "default",
					Annotations: map[string]string{
						coreeks.EKSClusterNameAnnotationKey: "eks-override",
					},
				},
				Spec: clusterv1.ClusterSpec{
					ControlPlaneRef: clusterv1.ContractVersionedObjectReference{
						APIGroup: kany8sControlPlaneAPIGroup,
						Kind:     kany8sControlPlaneKind,
						Name:     "demo-cp",
					},
				},
			},
			wantCAPIName: "demo",
			wantEKSName:  "eks-override",
			wantACKName:  "eks-override",
		},
		{
			name: "ack annotation override is applied last",
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "demo",
					Namespace: "default",
					Annotations: map[string]string{
						coreeks.EKSClusterNameAnnotationKey: "eks-override",
						coreeks.ACKClusterNameAnnotationKey: "ack-override",
					},
				},
			},
			wantCAPIName: "demo",
			wantEKSName:  "eks-override",
			wantACKName:  "ack-override",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotCAPIName, gotEKSName, gotACKName := resolveClusterNames(tt.cluster)
			if gotCAPIName != tt.wantCAPIName {
				t.Fatalf("capiClusterName = %q, want %q", gotCAPIName, tt.wantCAPIName)
			}
			if gotEKSName != tt.wantEKSName {
				t.Fatalf("eksClusterName = %q, want %q", gotEKSName, tt.wantEKSName)
			}
			if gotACKName != tt.wantACKName {
				t.Fatalf("ackClusterName = %q, want %q", gotACKName, tt.wantACKName)
			}
		})
	}
}

func TestEKSKubeconfigRotatorReconciler_ReconcileUsesControlPlaneRefNameAsDefaultEKSClusterName(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))

	now := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)
	expiration := now.Add(14 * time.Minute)
	caData := base64.StdEncoding.EncodeToString([]byte("ca-cert"))

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo-cluster",
			Namespace: "default",
			UID:       "cluster-uid",
			Annotations: map[string]string{
				coreeks.EnableAnnotationKey: coreeks.EnableAnnotationValue,
				coreeks.RegionAnnotationKey: "ap-northeast-1",
			},
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: kany8sControlPlaneAPIGroup,
				Kind:     kany8sControlPlaneKind,
				Name:     "demo-control-plane",
			},
		},
	}
	ack := ackClusterObj("default", "demo-control-plane", "https://demo.example", caData, "ap-northeast-1")
	tokenGen := &capturingTokenGenerator{
		token:      "k8s-aws-v1.controlplane",
		expiration: expiration,
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, ack).Build()
	r := &EKSKubeconfigRotatorReconciler{
		Client:         c,
		Scheme:         scheme,
		TokenGenerator: tokenGen,
		Policy: coreeks.RequeuePolicy{
			RefreshBefore:  5 * time.Minute,
			MaxRefresh:     10 * time.Minute,
			FailureBackoff: 30 * time.Second,
		},
		Now: func() time.Time { return now },
	}

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: "demo-cluster", Namespace: "default"}})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if got, want := res.RequeueAfter, 9*time.Minute; got != want {
		t.Fatalf("RequeueAfter = %s, want %s", got, want)
	}
	if got, want := tokenGen.callCount, 1; got != want {
		t.Fatalf("token generator call count = %d, want %d", got, want)
	}
	if got, want := tokenGen.gotClusterName, "demo-control-plane"; got != want {
		t.Fatalf("token generator clusterName = %q, want %q", got, want)
	}
	if got, want := tokenGen.gotRegion, "ap-northeast-1"; got != want {
		t.Fatalf("token generator region = %q, want %q", got, want)
	}

	probeName, err := kubeconfig.SecretName("demo-cluster")
	if err != nil {
		t.Fatalf("SecretName() error = %v", err)
	}
	probe := &corev1.Secret{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: probeName}, probe); err != nil {
		t.Fatalf("get probe secret: %v", err)
	}
	if got, want := probe.Annotations[coreeks.EKSClusterNameAnnotationKey], "demo-control-plane"; got != want {
		t.Fatalf("probe eksClusterName annotation = %q, want %q", got, want)
	}
}

func TestEKSKubeconfigRotatorReconciler_MapACKClusterToCAPIClustersUsesControlPlaneRefDefault(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	utilruntime.Must(clusterv1.AddToScheme(scheme))

	matching := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-a",
			Namespace: "default",
			Annotations: map[string]string{
				coreeks.EnableAnnotationKey: coreeks.EnableAnnotationValue,
			},
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: kany8sControlPlaneAPIGroup,
				Kind:     kany8sControlPlaneKind,
				Name:     "cluster-a-cp",
			},
		},
	}
	nonMatchingKind := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-b",
			Namespace: "default",
			Annotations: map[string]string{
				coreeks.EnableAnnotationKey: coreeks.EnableAnnotationValue,
			},
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: kany8sControlPlaneAPIGroup,
				Kind:     "OtherControlPlane",
				Name:     "cluster-a-cp",
			},
		},
	}
	disabled := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-c",
			Namespace: "default",
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: kany8sControlPlaneAPIGroup,
				Kind:     kany8sControlPlaneKind,
				Name:     "cluster-a-cp",
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(matching, nonMatchingKind, disabled).Build()
	r := &EKSKubeconfigRotatorReconciler{Client: c}

	requests := r.mapACKClusterToCAPIClusters(context.Background(), ackClusterObj("default", "cluster-a-cp", "", "", "ap-northeast-1"))
	if got, want := len(requests), 1; got != want {
		t.Fatalf("mapped request count = %d, want %d", got, want)
	}
	if got, want := requests[0].NamespacedName, (client.ObjectKey{Namespace: "default", Name: "cluster-a"}); got != want {
		t.Fatalf("mapped request = %v, want %v", got, want)
	}

	// Namespace mismatch should not map (the controller lists CAPI clusters within the ACK object's namespace).
	requestsOtherNS := r.mapACKClusterToCAPIClusters(context.Background(), ackClusterObj("other", "cluster-a-cp", "", "", "us-west-2"))
	if got, want := len(requestsOtherNS), 0; got != want {
		t.Fatalf("mapped request count (other namespace) = %d, want %d", got, want)
	}
}

func ackClusterObj(namespace, name, endpoint, caData, region string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(ackClusterGVK)
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.Object = map[string]any{
		"apiVersion": ackClusterGVK.GroupVersion().String(),
		"kind":       ackClusterGVK.Kind,
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
			"annotations": map[string]any{
				coreeks.ACKRegionMetadataAnnotationKey: region,
			},
		},
		"status": map[string]any{
			"endpoint": endpoint,
			"certificateAuthority": map[string]any{
				"data": caData,
			},
			"ackResourceMetadata": map[string]any{
				"region": region,
			},
		},
	}
	return obj
}
