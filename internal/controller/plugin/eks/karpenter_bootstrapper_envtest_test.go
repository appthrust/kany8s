package eks

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	memorycache "k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestEKSKarpenterBootstrapperReconciler_Envtest_DerivedResourcesAndReadyGate(t *testing.T) {
	t.Parallel()

	h := startEKSEnvtestHarness(
		t,
		stubCAPIClusterCRD(),
		stubNamespacedCRD("eks.services.k8s.aws", "v1alpha1", "Cluster", "clusters"),
		stubNamespacedCRD("eks.services.k8s.aws", "v1alpha1", "AccessEntry", "accessentries"),
		stubNamespacedCRD("eks.services.k8s.aws", "v1alpha1", "FargateProfile", "fargateprofiles"),
		stubNamespacedCRD("iam.services.k8s.aws", "v1alpha1", "Policy", "policies"),
		stubNamespacedCRD("iam.services.k8s.aws", "v1alpha1", "Role", "roles"),
		stubNamespacedCRD("iam.services.k8s.aws", "v1alpha1", "InstanceProfile", "instanceprofiles"),
		stubNamespacedCRD("iam.services.k8s.aws", "v1alpha1", "OpenIDConnectProvider", "openidconnectproviders"),
		stubNamespacedCRD("source.toolkit.fluxcd.io", "v1beta2", "OCIRepository", "ocirepositories"),
		stubNamespacedCRD("helm.toolkit.fluxcd.io", "v2", "HelmRelease", "helmreleases"),
		stubNamespacedCRD("addons.cluster.x-k8s.io", "v1beta2", "ClusterResourceSet", "clusterresourcesets"),
	)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
			Labels: map[string]string{
				karpenterEnableLabelKey: karpenterEnableLabelValue,
			},
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneRef: clusterv1.ContractVersionedObjectReference{
				APIGroup: kany8sControlPlaneAPIGroup,
				Kind:     kany8sControlPlaneKind,
				Name:     "demo-cp",
			},
			Topology: clusterv1.Topology{
				ClassRef: clusterv1.ClusterClassRef{
					Name: "kany8s-eks-byo",
				},
				Version: "v1.35.0",
				Variables: []clusterv1.ClusterVariable{
					clusterVariableJSON(t, topologyControlPlaneSubnetIDsVariableName, []string{"subnet-cp-a", "subnet-cp-b"}),
					clusterVariableJSON(t, topologyNodeSubnetIDsVariableName, []string{"subnet-a", "subnet-b"}),
					clusterVariableJSON(t, topologyControlPlaneSecurityGroupIDsVariableName, []string{"sg-control"}),
					clusterVariableJSON(t, topologyNodeSecurityGroupIDsVariableName, []string{"sg-node"}),
				},
			},
		},
	}
	if err := h.client.Create(context.Background(), cluster); err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	ack := &unstructured.Unstructured{}
	ack.SetGroupVersionKind(ackClusterGVK)
	ack.SetNamespace("default")
	ack.SetName("demo-cp")
	ack.Object = map[string]any{
		"apiVersion": ackClusterGVK.GroupVersion().String(),
		"kind":       ackClusterGVK.Kind,
		"metadata": map[string]any{
			"name":      "demo-cp",
			"namespace": "default",
			"annotations": map[string]any{
				"services.k8s.aws/region": "ap-northeast-1",
			},
		},
		"spec": map[string]any{
			"accessConfig": map[string]any{
				"authenticationMode": "API_AND_CONFIG_MAP",
			},
		},
		"status": map[string]any{
			"endpoint": "https://demo.example",
			"identity": map[string]any{
				"oidc": map[string]any{
					"issuer": "https://oidc.eks.ap-northeast-1.amazonaws.com/id/EXAMPLE",
				},
			},
			"ackResourceMetadata": map[string]any{
				"ownerAccountID": "123456789012",
				"region":         "ap-northeast-1",
			},
			"status": "ACTIVE",
		},
	}
	if err := h.client.Create(context.Background(), ack); err != nil {
		t.Fatalf("create ACK cluster: %v", err)
	}

	r := &EKSKarpenterBootstrapperReconciler{
		Client:             h.client,
		Scheme:             h.scheme,
		FailureBackoff:     25 * time.Second,
		SteadyStateRequeue: 3 * time.Minute,
		ValidateSubnets: func(context.Context, string, []string) (fargateSubnetValidationResult, error) {
			return fargateSubnetValidationResult{}, nil
		},
	}

	req := ctrl.Request{NamespacedName: client.ObjectKey{Namespace: "default", Name: "demo"}}
	first, err := r.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}
	if got, want := first.RequeueAfter, 25*time.Second; got != want {
		t.Fatalf("first RequeueAfter = %s, want %s", got, want)
	}

	for _, tc := range []struct {
		gvk  schema.GroupVersionKind
		name string
	}{
		{gvk: ackOIDCProviderGVK, name: "demo-oidc-provider"},
		{gvk: ackIAMPolicyGVK, name: "demo-karpenter-controller-policy"},
		{gvk: ackIAMRoleGVK, name: "demo-karpenter-controller"},
		{gvk: ackIAMInstanceProfileGVK, name: "demo-karpenter-node-instance-profile"},
		{gvk: ackAccessEntryGVK, name: "demo-karpenter-node"},
		// FargateProfile (= demo-karpenter / demo-coredns) is owned by the
		// kany8s-eks-byo ClusterClass RGD now, not the plugin (= APTH-1568
		// Path α). The simulation seed below stands in for the RGD output.
		{gvk: fluxHelmReleaseGVK, name: "demo-karpenter"},
		{gvk: clusterResourceSetGVK, name: "demo-karpenter-nodepool"},
	} {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(tc.gvk)
		if err := h.client.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: tc.name}, obj); err != nil {
			t.Fatalf("get %s %s: %v", tc.gvk.String(), tc.name, err)
		}
	}
	cm := &corev1.ConfigMap{}
	if err := h.client.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "demo-karpenter-nodepool"}, cm); err != nil {
		t.Fatalf("get ConfigMap demo-karpenter-nodepool: %v", err)
	}

	// Seed RGD-managed FargateProfile objects so the plugin's status monitor
	// (= isACKFargateProfileActive) can observe them. In production the
	// kany8s-eks-byo ClusterClass RGD creates these via ACK; envtest stands
	// in here.
	seedRGDFargateProfile(t, h.client, "default", "demo-karpenter")
	seedRGDFargateProfile(t, h.client, "default", "demo-coredns")
	markFargateProfileStatusActive(t, h.client, "default", "demo-karpenter")
	markFargateProfileStatusActive(t, h.client, "default", "demo-coredns")

	second, err := r.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
	if got, want := second.RequeueAfter, 3*time.Minute; got != want {
		t.Fatalf("second RequeueAfter = %s, want %s", got, want)
	}
}

func TestEKSKarpenterBootstrapperReconciler_Envtest_IsAPIAvailableDetectsCRDsAddedAfterStartup(t *testing.T) {
	t.Parallel()

	h := startEKSEnvtestHarness(
		t,
		stubCAPIClusterCRD(),
	)

	httpClient, err := rest.HTTPClientFor(h.cfg)
	if err != nil {
		t.Fatalf("rest HTTPClientFor: %v", err)
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfigAndClient(h.cfg, httpClient)
	if err != nil {
		t.Fatalf("new discovery client: %v", err)
	}
	// The reconciler consults a RESTMapper; wrap discovery with a deferred
	// mapper whose cache we can invalidate after late CRD installs so the
	// test mirrors how controller-runtime behaves in-cluster.
	cachedDiscovery := memorycache.NewMemCacheClient(discoveryClient)
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(cachedDiscovery)

	r := &EKSKarpenterBootstrapperReconciler{
		RESTMapper: mapper,
	}

	if r.isAPIAvailable(fluxOCIRepositoryGVK) {
		t.Fatalf("OCIRepository API unexpectedly available before CRD install")
	}
	if r.isAPIAvailable(fluxHelmReleaseGVK) {
		t.Fatalf("HelmRelease API unexpectedly available before CRD install")
	}

	crdClient, err := apiextensionsclientset.NewForConfig(h.cfg)
	if err != nil {
		t.Fatalf("new apiextensions client: %v", err)
	}
	installStubCRDs(
		t,
		crdClient,
		stubNamespacedCRD("source.toolkit.fluxcd.io", "v1beta2", "OCIRepository", "ocirepositories"),
		stubNamespacedCRD("helm.toolkit.fluxcd.io", "v2", "HelmRelease", "helmreleases"),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 10*time.Second, true, func(context.Context) (bool, error) {
		// Drop the discovery cache so newly-installed CRDs are re-fetched.
		cachedDiscovery.Invalidate()
		mapper.Reset()
		return r.areAPIsAvailable(fluxOCIRepositoryGVK, fluxHelmReleaseGVK), nil
	}); err != nil {
		t.Fatalf("Flux APIs were not detected after CRD install: %v", err)
	}
}

func clusterVariableJSON(t *testing.T, name string, value any) clusterv1.ClusterVariable {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal cluster variable %q: %v", name, err)
	}
	return clusterv1.ClusterVariable{
		Name: name,
		Value: apiextensionsv1.JSON{
			Raw: raw,
		},
	}
}

func markFargateProfileStatusActive(t *testing.T, c client.Client, namespace, name string) {
	t.Helper()
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(ackFargateProfileGVK)
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: name}, obj); err != nil {
		t.Fatalf("get FargateProfile %s/%s: %v", namespace, name, err)
	}
	if err := unstructured.SetNestedField(obj.Object, "ACTIVE", "status", "status"); err != nil {
		t.Fatalf("set status.status on %s/%s: %v", namespace, name, err)
	}
	if err := c.Update(context.Background(), obj); err != nil {
		t.Fatalf("update FargateProfile %s/%s: %v", namespace, name, err)
	}
}

// seedRGDFargateProfile materialises an ACK FargateProfile object that, in
// production, would be created by the kany8s-eks-byo ClusterClass RGD. The
// plugin no longer creates FargateProfile resources (= APTH-1568 Path α);
// it only monitors their status. This helper stands in for the RGD output
// so envtest can drive the status monitor path deterministically.
func seedRGDFargateProfile(t *testing.T, c client.Client, namespace, name string) {
	t.Helper()
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(ackFargateProfileGVK)
	obj.SetNamespace(namespace)
	obj.SetName(name)
	obj.SetLabels(map[string]string{
		"kany8s.io/managed-by":          "clusterclass-rgd",
		"cluster.x-k8s.io/cluster-name": "demo",
	})
	if err := unstructured.SetNestedField(obj.Object, name, "spec", "name"); err != nil {
		t.Fatalf("set spec.name on RGD FargateProfile %s/%s: %v", namespace, name, err)
	}
	if err := unstructured.SetNestedField(obj.Object, map[string]any{"from": map[string]any{"name": "demo", "namespace": namespace}}, "spec", "clusterRef"); err != nil {
		t.Fatalf("set spec.clusterRef on RGD FargateProfile %s/%s: %v", namespace, name, err)
	}
	if err := c.Create(context.Background(), obj); err != nil {
		t.Fatalf("create RGD FargateProfile %s/%s: %v", namespace, name, err)
	}
}
