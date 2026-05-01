package eks

import (
	"bytes"
	"context"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	coreeks "github.com/reoring/kany8s/internal/plugin/eks"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// Shared test fixtures kept here so goconst stays happy.
	testClusterName     = "demo"
	testEKSNodeRoleName = "eks-demo-node"
)

func TestEKSKarpenterBootstrapperReconciler_EnsureClusterNameLabel_SetsLabelIfMissing(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	utilruntime.Must(clusterv1.AddToScheme(scheme))

	cluster := &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: testClusterName, Namespace: "default"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()

	r := &EKSKarpenterBootstrapperReconciler{Client: c, Scheme: scheme}

	got := &clusterv1.Cluster{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(cluster), got); err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	if err := r.ensureClusterNameLabel(context.Background(), got); err != nil {
		t.Fatalf("ensureClusterNameLabel() error = %v", err)
	}

	updated := &clusterv1.Cluster{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(cluster), updated); err != nil {
		t.Fatalf("get updated cluster: %v", err)
	}
	if updated.Labels == nil {
		t.Fatalf("updated cluster has no labels")
	}
	if got, want := updated.Labels[capiClusterNameLabelKey], testClusterName; got != want {
		t.Fatalf("label %q = %q, want %q", capiClusterNameLabelKey, got, want)
	}
}

func TestEKSKarpenterBootstrapperReconciler_EnsureTopologyStringSliceVariable_PatchesValue(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	utilruntime.Must(clusterv1.AddToScheme(scheme))

	sgVar := clusterv1.ClusterVariable{Name: "vpc-security-group-ids"}
	sgVar.Value.Raw = []byte("[]")

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: testClusterName, Namespace: "default"},
		Spec: clusterv1.ClusterSpec{
			Topology: clusterv1.Topology{
				ClassRef:  clusterv1.ClusterClassRef{Name: "kany8s-eks-byo"},
				Version:   "v1.35.0",
				Variables: []clusterv1.ClusterVariable{sgVar},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
	r := &EKSKarpenterBootstrapperReconciler{Client: c, Scheme: scheme}

	got := &clusterv1.Cluster{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(cluster), got); err != nil {
		t.Fatalf("get cluster: %v", err)
	}

	patched, err := r.ensureTopologyStringSliceVariable(context.Background(), got, "vpc-security-group-ids", []string{" sg-123 "})
	if err != nil {
		t.Fatalf("ensureTopologyStringSliceVariable() error = %v", err)
	}
	if !patched {
		t.Fatalf("ensureTopologyStringSliceVariable() patched = false, want true")
	}

	updated := &clusterv1.Cluster{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(cluster), updated); err != nil {
		t.Fatalf("get updated cluster: %v", err)
	}
	ids, ok, err := readTopologyStringSlice(updated, "vpc-security-group-ids")
	if err != nil {
		t.Fatalf("readTopologyStringSlice() error = %v", err)
	}
	if !ok {
		t.Fatalf("readTopologyStringSlice() ok = false, want true")
	}
	if got, want := len(ids), 1; got != want {
		t.Fatalf("ids len = %d, want %d", got, want)
	}
	if got, want := ids[0], "sg-123"; got != want {
		t.Fatalf("ids[0] = %q, want %q", got, want)
	}
}

func TestBuildDefaultNodePoolYAML_IsValidMultiDocYAML(t *testing.T) {
	t.Parallel()

	yamlText := buildDefaultNodePoolYAML(
		"eks-demo",
		"eks-demo-node-profile",
		[]string{"subnet-a", "subnet-b"},
		[]string{"sg-1111", "sg-2222"},
	)

	decoder := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(yamlText)), 4096)
	objs := []unstructured.Unstructured{}
	for {
		var obj unstructured.Unstructured
		err := decoder.Decode(&obj)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decode YAML: %v", err)
		}
		if len(obj.Object) == 0 {
			continue
		}
		objs = append(objs, obj)
	}

	if got, want := len(objs), 2; got != want {
		t.Fatalf("decoded object count = %d, want %d", got, want)
	}
	if got, want := objs[0].GetAPIVersion(), "karpenter.k8s.aws/v1"; got != want {
		t.Fatalf("obj[0] apiVersion = %q, want %q", got, want)
	}
	if got, want := objs[0].GetKind(), "EC2NodeClass"; got != want {
		t.Fatalf("obj[0] kind = %q, want %q", got, want)
	}

	if terms, found, err := unstructured.NestedSlice(objs[0].Object, "spec", "subnetSelectorTerms"); err != nil {
		t.Fatalf("get subnetSelectorTerms: %v", err)
	} else if !found {
		t.Fatalf("missing subnetSelectorTerms")
	} else if got, want := len(terms), 2; got != want {
		t.Fatalf("subnetSelectorTerms len = %d, want %d", got, want)
	}
	if terms, found, err := unstructured.NestedSlice(objs[0].Object, "spec", "securityGroupSelectorTerms"); err != nil {
		t.Fatalf("get securityGroupSelectorTerms: %v", err)
	} else if !found {
		t.Fatalf("missing securityGroupSelectorTerms")
	} else if got, want := len(terms), 3; got != want {
		// 2 explicit `id` selectors (= sg-1111 + sg-2222) + 1 tag-based selector
		// for the EKS cluster security group (= aws:eks:cluster-name).
		t.Fatalf("securityGroupSelectorTerms len = %d, want %d", got, want)
	} else {
		// Last term must be the tag-based cluster SG selector.
		last, ok := terms[len(terms)-1].(map[string]interface{})
		if !ok {
			t.Fatalf("last securityGroupSelectorTerms entry is not a map: %T", terms[len(terms)-1])
		}
		tags, ok := last["tags"].(map[string]interface{})
		if !ok {
			t.Fatalf("last securityGroupSelectorTerms entry missing tags map: %v", last)
		}
		if got, want := tags["aws:eks:cluster-name"], "eks-demo"; got != want {
			t.Fatalf("tags[aws:eks:cluster-name] = %v, want %q", got, want)
		}
	}
	if got, found, err := unstructured.NestedString(objs[0].Object, "spec", "instanceProfile"); err != nil {
		t.Fatalf("get spec.instanceProfile: %v", err)
	} else if !found {
		t.Fatalf("missing spec.instanceProfile")
	} else if want := "eks-demo-node-profile"; got != want {
		t.Fatalf("spec.instanceProfile = %q, want %q", got, want)
	}
	if _, found, err := unstructured.NestedString(objs[0].Object, "spec", "role"); err != nil {
		t.Fatalf("get spec.role: %v", err)
	} else if found {
		t.Fatalf("unexpected spec.role in EC2NodeClass")
	}

	if got, want := objs[1].GetAPIVersion(), "karpenter.sh/v1"; got != want {
		t.Fatalf("obj[1] apiVersion = %q, want %q", got, want)
	}
	if got, want := objs[1].GetKind(), "NodePool"; got != want {
		t.Fatalf("obj[1] kind = %q, want %q", got, want)
	}
}

func TestEKSKarpenterBootstrapperReconciler_EnsureDefaultNodePoolResources_CreatesCRSWhenConfigMapAlreadyDesired(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testClusterName,
			Namespace: "default",
			UID:       "cluster-uid",
			Labels: map[string]string{
				karpenterEnableLabelKey: karpenterEnableLabelValue,
				capiClusterNameLabelKey: testClusterName,
			},
		},
	}

	desiredYAML := buildDefaultNodePoolYAML(
		"eks-demo",
		"eks-demo-node-profile",
		[]string{"subnet-a", "subnet-b"},
		[]string{"sg-1111"},
	)

	cmName := "demo-karpenter-nodepool"
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: "default"}}
	mutateManagedConfigMap(cm, cluster, desiredYAML)
	if err := controllerutil.SetOwnerReference(cluster, cm, scheme); err != nil {
		t.Fatalf("set owner ref: %v", err)
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, cm).Build()
	r := &EKSKarpenterBootstrapperReconciler{Client: c, Scheme: scheme}

	if err := r.ensureDefaultNodePoolResources(
		context.Background(),
		cluster,
		testClusterName,
		"eks-demo",
		"eks-demo-node-profile",
		[]string{"subnet-a", "subnet-b"},
		[]string{"sg-1111"},
	); err != nil {
		t.Fatalf("ensureDefaultNodePoolResources() error = %v", err)
	}

	crs := &unstructured.Unstructured{}
	crs.SetGroupVersionKind(clusterResourceSetGVK)
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "demo-karpenter-nodepool"}, crs); err != nil {
		t.Fatalf("get ClusterResourceSet: %v", err)
	}

	matchLabels, found, err := unstructured.NestedMap(crs.Object, "spec", "clusterSelector", "matchLabels")
	if err != nil {
		t.Fatalf("get clusterSelector.matchLabels: %v", err)
	}
	if !found {
		t.Fatalf("missing clusterSelector.matchLabels")
	}
	if got, want := matchLabels[capiClusterNameLabelKey], testClusterName; got != want {
		t.Fatalf("matchLabels[%q] = %v, want %q", capiClusterNameLabelKey, got, want)
	}
	if got, want := matchLabels[karpenterEnableLabelKey], karpenterEnableLabelValue; got != want {
		t.Fatalf("matchLabels[%q] = %v, want %q", karpenterEnableLabelKey, got, want)
	}

	resources, found, err := unstructured.NestedSlice(crs.Object, "spec", "resources")
	if err != nil {
		t.Fatalf("get spec.resources: %v", err)
	}
	if !found {
		t.Fatalf("missing spec.resources")
	}
	if got, want := len(resources), 1; got != want {
		t.Fatalf("spec.resources len = %d, want %d", got, want)
	}
	first, ok := resources[0].(map[string]any)
	if !ok {
		t.Fatalf("spec.resources[0] has unexpected type %T", resources[0])
	}
	if got, want := first["kind"], "ConfigMap"; got != want {
		t.Fatalf("spec.resources[0].kind = %v, want %q", got, want)
	}
	if got, want := first["name"], cmName; got != want {
		t.Fatalf("spec.resources[0].name = %v, want %q", got, want)
	}

	strategy, found, err := unstructured.NestedString(crs.Object, "spec", "strategy")
	if err != nil {
		t.Fatalf("get spec.strategy: %v", err)
	}
	if !found {
		t.Fatalf("missing spec.strategy")
	}
	if got, want := strategy, "Reconcile"; got != want {
		t.Fatalf("spec.strategy = %q, want %q", got, want)
	}
}

func TestBuildKarpenterControllerPolicyDocument_InterpolatesAndIsValidJSON(t *testing.T) {
	t.Parallel()

	policyText := buildKarpenterControllerPolicyDocument(
		" ap-northeast-1 ",
		" 123456789012 ",
		" demo-eks ",
		" arn:aws:iam::123456789012:role/demo-eks-node ",
	)

	var policy map[string]any
	if err := json.Unmarshal([]byte(policyText), &policy); err != nil {
		t.Fatalf("policy json unmarshal error: %v", err)
	}

	if got, ok := policy["Version"].(string); !ok || got != "2012-10-17" {
		t.Fatalf("Version = %v, want %q", policy["Version"], "2012-10-17")
	}

	if !strings.Contains(policyText, "arn:aws:eks:ap-northeast-1:123456789012:cluster/demo-eks") {
		t.Fatalf("policy does not contain interpolated EKS cluster ARN: %s", policyText)
	}
	if !strings.Contains(policyText, "arn:aws:iam::123456789012:instance-profile/*") {
		t.Fatalf("policy does not contain interpolated account ID for instance profiles: %s", policyText)
	}

	statements, ok := policy["Statement"].([]any)
	if !ok || len(statements) == 0 {
		t.Fatalf("Statement has unexpected type/value: %#v", policy["Statement"])
	}

	var passRole map[string]any
	for _, s := range statements {
		stmt, ok := s.(map[string]any)
		if !ok {
			continue
		}
		if sid, _ := stmt["Sid"].(string); sid == "AllowPassingInstanceRole" {
			passRole = stmt
			break
		}
	}
	if passRole == nil {
		t.Fatalf("statement with Sid=AllowPassingInstanceRole not found")
	}

	resources, ok := passRole["Resource"].([]any)
	if !ok || len(resources) != 1 {
		t.Fatalf("AllowPassingInstanceRole.Resource has unexpected value: %#v", passRole["Resource"])
	}
	if got, want := resources[0], "arn:aws:iam::123456789012:role/demo-eks-node"; got != want {
		t.Fatalf("AllowPassingInstanceRole.Resource[0] = %v, want %q", got, want)
	}
}

// nolint:gocyclo
func TestEKSKarpenterBootstrapperReconciler_EnsureACKResources_CreateExpectedSpecs(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	utilruntime.Must(clusterv1.AddToScheme(scheme))

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testClusterName,
			Namespace: "default",
			UID:       "cluster-uid",
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
	r := &EKSKarpenterBootstrapperReconciler{Client: c, Scheme: scheme}

	if ok, err := r.ensureOIDCProvider(context.Background(), cluster, "demo-oidc-provider", "ap-northeast-1", "https://oidc.eks.ap-northeast-1.amazonaws.com/id/EXAMPLE", []string{"thumbprint1"}); err != nil {
		t.Fatalf("ensureOIDCProvider() error = %v", err)
	} else if !ok {
		t.Fatalf("ensureOIDCProvider() managed = false, want true")
	}
	oidc := getUnstructured(t, c, ackOIDCProviderGVK, "demo-oidc-provider")
	if got, _, err := unstructured.NestedString(oidc.Object, "spec", "url"); err != nil {
		t.Fatalf("oidc spec.url: %v", err)
	} else if want := "https://oidc.eks.ap-northeast-1.amazonaws.com/id/EXAMPLE"; got != want {
		t.Fatalf("oidc spec.url = %q, want %q", got, want)
	}
	if got, _, err := unstructured.NestedStringSlice(oidc.Object, "spec", "clientIDs"); err != nil {
		t.Fatalf("oidc spec.clientIDs: %v", err)
	} else if len(got) != 1 || got[0] != "sts.amazonaws.com" {
		t.Fatalf("oidc spec.clientIDs = %#v, want [\"sts.amazonaws.com\"]", got)
	}
	if got, _, err := unstructured.NestedStringSlice(oidc.Object, "spec", "thumbprints"); err != nil {
		t.Fatalf("oidc spec.thumbprints: %v", err)
	} else if len(got) != 1 || got[0] != "thumbprint1" {
		t.Fatalf("oidc spec.thumbprints = %#v, want [\"thumbprint1\"]", got)
	}

	if ok, err := r.ensureIAMPolicy(context.Background(), cluster, "demo-karpenter-controller-policy", "ap-northeast-1", `{"Version":"2012-10-17","Statement":[]}`, "eks-demo-karpenter-controller"); err != nil {
		t.Fatalf("ensureIAMPolicy() error = %v", err)
	} else if !ok {
		t.Fatalf("ensureIAMPolicy() managed = false, want true")
	}
	policy := getUnstructured(t, c, ackIAMPolicyGVK, "demo-karpenter-controller-policy")
	if got, _, err := unstructured.NestedString(policy.Object, "spec", "name"); err != nil {
		t.Fatalf("policy spec.name: %v", err)
	} else if want := "eks-demo-karpenter-controller"; got != want {
		t.Fatalf("policy spec.name = %q, want %q", got, want)
	}
	if tags, found, err := unstructured.NestedSlice(policy.Object, "spec", "tags"); err != nil {
		t.Fatalf("policy spec.tags: %v", err)
	} else if !found || len(tags) == 0 {
		t.Fatalf("policy spec.tags missing")
	} else if got, want := findTagValue(t, tags, "kany8s.io/managed-by"), karpenterManagedByValue; got != want {
		t.Fatalf("policy spec.tags managed-by = %q, want %q", got, want)
	}

	if ok, err := r.ensureIAMRoleForIRSA(context.Background(), cluster, "demo-karpenter-controller", "ap-northeast-1", "eks-demo-karpenter-controller", "assume-irsa", []string{"demo-karpenter-controller-policy"}); err != nil {
		t.Fatalf("ensureIAMRoleForIRSA() error = %v", err)
	} else if !ok {
		t.Fatalf("ensureIAMRoleForIRSA() managed = false, want true")
	}
	irsaRole := getUnstructured(t, c, ackIAMRoleGVK, "demo-karpenter-controller")
	if got, _, err := unstructured.NestedString(irsaRole.Object, "spec", "name"); err != nil {
		t.Fatalf("irsa role spec.name: %v", err)
	} else if want := "eks-demo-karpenter-controller"; got != want {
		t.Fatalf("irsa role spec.name = %q, want %q", got, want)
	}
	if got, _, err := unstructured.NestedString(irsaRole.Object, "spec", "assumeRolePolicyDocument"); err != nil {
		t.Fatalf("irsa role spec.assumeRolePolicyDocument: %v", err)
	} else if want := "assume-irsa"; got != want {
		t.Fatalf("irsa role spec.assumeRolePolicyDocument = %q, want %q", got, want)
	}
	if refs, _, err := unstructured.NestedSlice(irsaRole.Object, "spec", "policyRefs"); err != nil {
		t.Fatalf("irsa role spec.policyRefs: %v", err)
	} else if len(refs) != 1 {
		t.Fatalf("irsa role spec.policyRefs len = %d, want 1", len(refs))
	}

	if ok, err := r.ensureIAMRoleForEC2(context.Background(), cluster, "demo-karpenter-node", "ap-northeast-1", testEKSNodeRoleName, []string{"arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy"}); err != nil {
		t.Fatalf("ensureIAMRoleForEC2() error = %v", err)
	} else if !ok {
		t.Fatalf("ensureIAMRoleForEC2() managed = false, want true")
	}
	ec2Role := getUnstructured(t, c, ackIAMRoleGVK, "demo-karpenter-node")
	if got, _, err := unstructured.NestedString(ec2Role.Object, "spec", "name"); err != nil {
		t.Fatalf("ec2 role spec.name: %v", err)
	} else if want := testEKSNodeRoleName; got != want {
		t.Fatalf("ec2 role spec.name = %q, want %q", got, want)
	}
	if got, _, err := unstructured.NestedStringSlice(ec2Role.Object, "spec", "policies"); err != nil {
		t.Fatalf("ec2 role spec.policies: %v", err)
	} else if len(got) != 1 || got[0] != "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy" {
		t.Fatalf("ec2 role spec.policies = %#v", got)
	}
	if ok, err := r.ensureIAMInstanceProfile(context.Background(), cluster, "demo-karpenter-node-instance-profile", "ap-northeast-1", testEKSNodeRoleName, "demo-karpenter-node"); err != nil {
		t.Fatalf("ensureIAMInstanceProfile() error = %v", err)
	} else if !ok {
		t.Fatalf("ensureIAMInstanceProfile() managed = false, want true")
	}
	instanceProfile := getUnstructured(t, c, ackIAMInstanceProfileGVK, "demo-karpenter-node-instance-profile")
	if got, _, err := unstructured.NestedString(instanceProfile.Object, "spec", "name"); err != nil {
		t.Fatalf("instanceProfile spec.name: %v", err)
	} else if want := testEKSNodeRoleName; got != want {
		t.Fatalf("instanceProfile spec.name = %q, want %q", got, want)
	}
	if got, _, err := unstructured.NestedString(instanceProfile.Object, "spec", "roleRef", "from", "name"); err != nil {
		t.Fatalf("instanceProfile spec.roleRef.from.name: %v", err)
	} else if want := "demo-karpenter-node"; got != want {
		t.Fatalf("instanceProfile spec.roleRef.from.name = %q, want %q", got, want)
	}
	if got, _, err := unstructured.NestedString(instanceProfile.Object, "spec", "roleRef", "from", "namespace"); err != nil {
		t.Fatalf("instanceProfile spec.roleRef.from.namespace: %v", err)
	} else if want := "default"; got != want {
		t.Fatalf("instanceProfile spec.roleRef.from.namespace = %q, want %q", got, want)
	}

	// Fargate Pod Execution IAM Role + Fargate Profile are produced declaratively by
	// the kany8s-eks-byo ClusterClass RGD now (see knowledge/eks-root-fix-design.md
	// and APTH-1568 Path α). The plugin no longer creates them, so the create-spec
	// assertions that previously sat here are removed; status monitoring
	// (isACKFargateProfileActive) is exercised separately in the envtest.

	if ok, err := r.ensureAccessEntry(context.Background(), cluster, "demo-karpenter-node", "ap-northeast-1", "demo-ack", "arn:aws:iam::123456789012:role/demo-node"); err != nil {
		t.Fatalf("ensureAccessEntry() error = %v", err)
	} else if !ok {
		t.Fatalf("ensureAccessEntry() managed = false, want true")
	}
	accessEntry := getUnstructured(t, c, ackAccessEntryGVK, "demo-karpenter-node")
	if got, _, err := unstructured.NestedString(accessEntry.Object, "spec", "principalARN"); err != nil {
		t.Fatalf("accessEntry spec.principalARN: %v", err)
	} else if want := "arn:aws:iam::123456789012:role/demo-node"; got != want {
		t.Fatalf("accessEntry spec.principalARN = %q, want %q", got, want)
	}
	if got, _, err := unstructured.NestedString(accessEntry.Object, "spec", "type"); err != nil {
		t.Fatalf("accessEntry spec.type: %v", err)
	} else if want := "EC2_LINUX"; got != want {
		t.Fatalf("accessEntry spec.type = %q, want %q", got, want)
	}
	if got, _, err := unstructured.NestedString(accessEntry.Object, "spec", "clusterRef", "from", "name"); err != nil {
		t.Fatalf("accessEntry spec.clusterRef.from.name: %v", err)
	} else if want := "demo-ack"; got != want {
		t.Fatalf("accessEntry spec.clusterRef.from.name = %q, want %q", got, want)
	}
	if tags, found, err := unstructured.NestedMap(accessEntry.Object, "spec", "tags"); err != nil {
		t.Fatalf("accessEntry spec.tags: %v", err)
	} else if !found {
		t.Fatalf("accessEntry spec.tags missing")
	} else if got, want := tags["kany8s.io/managed-by"], karpenterManagedByValue; got != want {
		t.Fatalf("accessEntry spec.tags managed-by = %v, want %q", got, want)
	}

	// Fargate Profile create-spec assertions removed: the kany8s-eks-byo
	// ClusterClass RGD now owns FargateProfile lifecycle (PR-1, APTH-1568).
	// Status monitoring is covered by the envtest in this same package.
}

func TestEKSKarpenterBootstrapperReconciler_EnsureIAMRoleForEC2_TakeoverWhenExplicitlyAllowed(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	utilruntime.Must(clusterv1.AddToScheme(scheme))

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testClusterName,
			Namespace: "default",
			UID:       "cluster-uid",
			Annotations: map[string]string{
				coreeks.AllowUnmanagedTakeoverAnnotationKey: coreeks.AllowUnmanagedTakeoverAnnotationValue,
			},
		},
	}

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(ackIAMRoleGVK)
	existing.SetNamespace("default")
	existing.SetName("demo-karpenter-node")
	mustSetNestedField(existing, "legacy-role", "spec", "name")

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, existing).Build()
	r := &EKSKarpenterBootstrapperReconciler{Client: c, Scheme: scheme}

	ok, err := r.ensureIAMRoleForEC2(
		context.Background(),
		cluster,
		"demo-karpenter-node",
		"ap-northeast-1",
		testEKSNodeRoleName,
		[]string{"arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy"},
	)
	if err != nil {
		t.Fatalf("ensureIAMRoleForEC2() error = %v", err)
	}
	if !ok {
		t.Fatalf("ensureIAMRoleForEC2() managed = false, want true")
	}

	got := getUnstructured(t, c, ackIAMRoleGVK, "demo-karpenter-node")
	if gotAnn := got.GetAnnotations()[coreeks.ManagedByAnnotationKey]; gotAnn != karpenterManagedByValue {
		t.Fatalf("managed annotation = %q, want %q", gotAnn, karpenterManagedByValue)
	}
	if gotName, _, err := unstructured.NestedString(got.Object, "spec", "name"); err != nil {
		t.Fatalf("role spec.name: %v", err)
	} else if want := testEKSNodeRoleName; gotName != want {
		t.Fatalf("role spec.name = %q, want %q", gotName, want)
	}
}

func TestEKSKarpenterBootstrapperReconciler_EnsureIAMPolicy_InvalidExistingShapeReturnsError(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	utilruntime.Must(clusterv1.AddToScheme(scheme))

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testClusterName,
			Namespace: "default",
			UID:       "cluster-uid",
		},
	}

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(ackIAMPolicyGVK)
	existing.SetNamespace("default")
	existing.SetName("demo-karpenter-controller-policy")
	setManagedBy(existing)
	existing.Object["spec"] = "invalid-type"

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, existing).Build()
	r := &EKSKarpenterBootstrapperReconciler{Client: c, Scheme: scheme}

	ok, err := r.ensureIAMPolicy(
		context.Background(),
		cluster,
		"demo-karpenter-controller-policy",
		"ap-northeast-1",
		`{"Version":"2012-10-17","Statement":[]}`,
		"eks-demo-karpenter-controller",
	)
	if err == nil {
		t.Fatalf("ensureIAMPolicy() error = nil, want non-nil")
	}
	if ok {
		t.Fatalf("ensureIAMPolicy() managed = true, want false on mutation error")
	}
	if !strings.Contains(err.Error(), "mutate object fields") {
		t.Fatalf("ensureIAMPolicy() error = %v, want mutation error", err)
	}
}

func TestEKSKarpenterBootstrapperReconciler_EnsureFluxKarpenter_CreateExpectedSpecs(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	utilruntime.Must(clusterv1.AddToScheme(scheme))

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testClusterName,
			Namespace: "default",
			UID:       "cluster-uid",
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
	r := &EKSKarpenterBootstrapperReconciler{Client: c, Scheme: scheme}

	values := defaultKarpenterHelmValues("eks-demo", "https://demo.example", "arn:aws:iam::123456789012:role/demo-karpenter-controller")
	ok, err := r.ensureFluxKarpenter(context.Background(), cluster, testClusterName, defaultKarpenterChartVersion, values)
	if err != nil {
		t.Fatalf("ensureFluxKarpenter() error = %v", err)
	}
	if !ok {
		t.Fatalf("ensureFluxKarpenter() managed = false, want true")
	}

	oci := getUnstructured(t, c, fluxOCIRepositoryGVK, "demo-karpenter")
	if got, _, err := unstructured.NestedString(oci.Object, "spec", "interval"); err != nil {
		t.Fatalf("OCIRepository spec.interval: %v", err)
	} else if want := "10m"; got != want {
		t.Fatalf("OCIRepository spec.interval = %q, want %q", got, want)
	}
	if got, _, err := unstructured.NestedString(oci.Object, "spec", "url"); err != nil {
		t.Fatalf("OCIRepository spec.url: %v", err)
	} else if want := "oci://public.ecr.aws/karpenter/karpenter"; got != want {
		t.Fatalf("OCIRepository spec.url = %q, want %q", got, want)
	}
	if got, _, err := unstructured.NestedString(oci.Object, "spec", "ref", "tag"); err != nil {
		t.Fatalf("OCIRepository spec.ref.tag: %v", err)
	} else if got != defaultKarpenterChartVersion {
		t.Fatalf("OCIRepository spec.ref.tag = %q, want %q", got, defaultKarpenterChartVersion)
	}

	hr := getUnstructured(t, c, fluxHelmReleaseGVK, "demo-karpenter")
	if got, _, err := unstructured.NestedString(hr.Object, "spec", "interval"); err != nil {
		t.Fatalf("HelmRelease spec.interval: %v", err)
	} else if want := "5m"; got != want {
		t.Fatalf("HelmRelease spec.interval = %q, want %q", got, want)
	}
	if got, _, err := unstructured.NestedString(hr.Object, "spec", "kubeConfig", "secretRef", "name"); err != nil {
		t.Fatalf("HelmRelease spec.kubeConfig.secretRef.name: %v", err)
	} else if want := "demo-kubeconfig"; got != want {
		t.Fatalf("HelmRelease spec.kubeConfig.secretRef.name = %q, want %q", got, want)
	}
	if got, _, err := unstructured.NestedString(hr.Object, "spec", "chartRef", "kind"); err != nil {
		t.Fatalf("HelmRelease spec.chartRef.kind: %v", err)
	} else if want := "OCIRepository"; got != want {
		t.Fatalf("HelmRelease spec.chartRef.kind = %q, want %q", got, want)
	}
	if got, _, err := unstructured.NestedString(hr.Object, "spec", "install", "crds"); err != nil {
		t.Fatalf("HelmRelease spec.install.crds: %v", err)
	} else if want := "CreateReplace"; got != want {
		t.Fatalf("HelmRelease spec.install.crds = %q, want %q", got, want)
	}
	if got, _, err := unstructured.NestedString(hr.Object, "spec", "upgrade", "crds"); err != nil {
		t.Fatalf("HelmRelease spec.upgrade.crds: %v", err)
	} else if want := "CreateReplace"; got != want {
		t.Fatalf("HelmRelease spec.upgrade.crds = %q, want %q", got, want)
	}
	if got, _, err := unstructured.NestedString(hr.Object, "spec", "values", "settings", "clusterName"); err != nil {
		t.Fatalf("HelmRelease spec.values.settings.clusterName: %v", err)
	} else if want := "eks-demo"; got != want {
		t.Fatalf("HelmRelease spec.values.settings.clusterName = %q, want %q", got, want)
	}
	if got, _, err := unstructured.NestedString(hr.Object, "spec", "values", "settings", "clusterEndpoint"); err != nil {
		t.Fatalf("HelmRelease spec.values.settings.clusterEndpoint: %v", err)
	} else if want := "https://demo.example"; got != want {
		t.Fatalf("HelmRelease spec.values.settings.clusterEndpoint = %q, want %q", got, want)
	}
	if got, _, err := unstructured.NestedString(hr.Object, "spec", "values", "serviceAccount", "annotations", "eks.amazonaws.com/role-arn"); err != nil {
		t.Fatalf("HelmRelease spec.values.serviceAccount.annotations role arn: %v", err)
	} else if want := "arn:aws:iam::123456789012:role/demo-karpenter-controller"; got != want {
		t.Fatalf("HelmRelease role arn = %q, want %q", got, want)
	}
	if got, _, err := unstructured.NestedBool(hr.Object, "spec", "values", "webhook", "enabled"); err != nil {
		t.Fatalf("HelmRelease spec.values.webhook.enabled: %v", err)
	} else if got {
		t.Fatalf("HelmRelease spec.values.webhook.enabled = true, want false")
	}
}

func TestEKSKarpenterBootstrapperReconciler_ResolveKarpenterChartTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		reconciler *EKSKarpenterBootstrapperReconciler
		cluster    *clusterv1.Cluster
		want       string
	}{
		{
			name:       "uses cluster annotation override",
			reconciler: &EKSKarpenterBootstrapperReconciler{KarpenterChartTag: "1.0.9"},
			cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						karpenterChartVersionAnnotation: "1.1.0",
					},
				},
			},
			want: "1.1.0",
		},
		{
			name:       "uses reconciler override",
			reconciler: &EKSKarpenterBootstrapperReconciler{KarpenterChartTag: "1.2.0"},
			cluster:    &clusterv1.Cluster{},
			want:       "1.2.0",
		},
		{
			name:       "falls back to default",
			reconciler: &EKSKarpenterBootstrapperReconciler{},
			cluster:    &clusterv1.Cluster{},
			want:       defaultKarpenterChartVersion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.reconciler.resolveKarpenterChartTag(tt.cluster); got != tt.want {
				t.Fatalf("resolveKarpenterChartTag() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEKSKarpenterBootstrapperReconciler_ResolveKarpenterHelmValues_OverrideAndProtectCriticalKeys(t *testing.T) {
	t.Parallel()

	r := &EKSKarpenterBootstrapperReconciler{}
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				karpenterInterruptionQueueAnnotation: "demo-interruption-queue",
				karpenterHelmValuesAnnotation: `{
  "webhook": {"enabled": true},
  "settings": {"clusterName": "bad", "clusterEndpoint": "https://bad"},
  "serviceAccount": {"annotations": {"extra": "yes", "eks.amazonaws.com/role-arn": "bad"}},
  "resources": {"requests": {"cpu": "250m"}}
}`,
			},
		},
	}

	values, err := r.resolveKarpenterHelmValues(cluster, "eks-demo", "https://demo.example", "arn:aws:iam::123456789012:role/demo")
	if err != nil {
		t.Fatalf("resolveKarpenterHelmValues() error = %v", err)
	}
	if got, _, err := unstructured.NestedString(values, "settings", "clusterName"); err != nil {
		t.Fatalf("settings.clusterName: %v", err)
	} else if want := "eks-demo"; got != want {
		t.Fatalf("settings.clusterName = %q, want %q", got, want)
	}
	if got, _, err := unstructured.NestedString(values, "settings", "clusterEndpoint"); err != nil {
		t.Fatalf("settings.clusterEndpoint: %v", err)
	} else if want := "https://demo.example"; got != want {
		t.Fatalf("settings.clusterEndpoint = %q, want %q", got, want)
	}
	if got, _, err := unstructured.NestedString(values, "serviceAccount", "annotations", "eks.amazonaws.com/role-arn"); err != nil {
		t.Fatalf("serviceAccount.annotations role-arn: %v", err)
	} else if want := "arn:aws:iam::123456789012:role/demo"; got != want {
		t.Fatalf("role-arn = %q, want %q", got, want)
	}
	if got, _, err := unstructured.NestedString(values, "serviceAccount", "annotations", "extra"); err != nil {
		t.Fatalf("serviceAccount.annotations.extra: %v", err)
	} else if want := "yes"; got != want {
		t.Fatalf("serviceAccount.annotations.extra = %q, want %q", got, want)
	}
	if got, _, err := unstructured.NestedBool(values, "webhook", "enabled"); err != nil {
		t.Fatalf("webhook.enabled: %v", err)
	} else if !got {
		t.Fatalf("webhook.enabled = false, want true")
	}
	if got, _, err := unstructured.NestedString(values, "settings", "interruptionQueue"); err != nil {
		t.Fatalf("settings.interruptionQueue: %v", err)
	} else if want := "demo-interruption-queue"; got != want {
		t.Fatalf("settings.interruptionQueue = %q, want %q", got, want)
	}
}

func TestEKSKarpenterBootstrapperReconciler_ResolveKarpenterHelmValues_InvalidJSON(t *testing.T) {
	t.Parallel()

	r := &EKSKarpenterBootstrapperReconciler{}
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				karpenterHelmValuesAnnotation: "{",
			},
		},
	}

	if _, err := r.resolveKarpenterHelmValues(cluster, "eks-demo", "https://demo.example", "arn:aws:iam::123456789012:role/demo"); err == nil {
		t.Fatalf("resolveKarpenterHelmValues() error = nil, want non-nil")
	}
}

func TestEKSKarpenterBootstrapperReconciler_ResolveOIDCThumbprints_AnnotationGate(t *testing.T) {
	t.Parallel()

	called := 0
	r := &EKSKarpenterBootstrapperReconciler{
		ResolveOIDCThumbprints: func(context.Context, string) ([]string, error) {
			called++
			return []string{"abc123"}, nil
		},
	}

	clusterDisabled := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{},
		},
	}
	got, err := r.resolveOIDCThumbprints(context.Background(), clusterDisabled, "https://issuer.example")
	if err != nil {
		t.Fatalf("resolveOIDCThumbprints(disabled) error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("resolveOIDCThumbprints(disabled) = %#v, want nil/empty", got)
	}
	if called != 0 {
		t.Fatalf("resolver called %d times, want 0", called)
	}

	clusterEnabled := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				oidcThumbprintAutoAnnotation: "enabled",
			},
		},
	}
	got, err = r.resolveOIDCThumbprints(context.Background(), clusterEnabled, "https://issuer.example")
	if err != nil {
		t.Fatalf("resolveOIDCThumbprints(enabled) error = %v", err)
	}
	if called != 1 {
		t.Fatalf("resolver called %d times, want 1", called)
	}
	if len(got) != 1 || got[0] != "abc123" {
		t.Fatalf("resolveOIDCThumbprints(enabled) = %#v, want [abc123]", got)
	}
}

func TestDefaultResolveOIDCThumbprints_UnverifiedChainReturnsSentinel(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(nil)
	t.Cleanup(server.Close)

	if _, err := defaultResolveOIDCThumbprints(context.Background(), server.URL); !errors.Is(err, errOIDCThumbprintUnverified) {
		t.Fatalf("defaultResolveOIDCThumbprints() error = %v, want sentinel %v", err, errOIDCThumbprintUnverified)
	}
}

func TestSelectTopIntermediateCACert_RootIncluded(t *testing.T) {
	t.Parallel()

	leaf := &x509.Certificate{
		Raw:     []byte("leaf"),
		IsCA:    false,
		Subject: pkix.Name{CommonName: "leaf"},
		Issuer:  pkix.Name{CommonName: "intermediate"},
	}
	intermediate := &x509.Certificate{
		Raw:     []byte("intermediate"),
		IsCA:    true,
		Subject: pkix.Name{CommonName: "intermediate"},
		Issuer:  pkix.Name{CommonName: "root"},
	}
	root := &x509.Certificate{
		Raw:     []byte("root"),
		IsCA:    true,
		Subject: pkix.Name{CommonName: "root"},
		Issuer:  pkix.Name{CommonName: "root"},
	}

	got, err := selectTopIntermediateCACert([]*x509.Certificate{leaf, intermediate, root})
	if err != nil {
		t.Fatalf("selectTopIntermediateCACert() error = %v", err)
	}
	if got != intermediate {
		t.Fatalf("selectTopIntermediateCACert() = %#v, want intermediate cert", got)
	}
}

func TestSelectTopIntermediateCACert_RootNotIncluded(t *testing.T) {
	t.Parallel()

	leaf := &x509.Certificate{
		Raw:     []byte("leaf"),
		IsCA:    false,
		Subject: pkix.Name{CommonName: "leaf"},
		Issuer:  pkix.Name{CommonName: "intermediate"},
	}
	intermediate := &x509.Certificate{
		Raw:     []byte("intermediate"),
		IsCA:    true,
		Subject: pkix.Name{CommonName: "intermediate"},
		Issuer:  pkix.Name{CommonName: "root"},
	}

	got, err := selectTopIntermediateCACert([]*x509.Certificate{leaf, intermediate})
	if err != nil {
		t.Fatalf("selectTopIntermediateCACert() error = %v", err)
	}
	if got != intermediate {
		t.Fatalf("selectTopIntermediateCACert() = %#v, want intermediate cert", got)
	}
}

func TestSelectTopIntermediateCACert_NoIntermediateReturnsSentinel(t *testing.T) {
	t.Parallel()

	leaf := &x509.Certificate{
		Raw:     []byte("leaf"),
		IsCA:    false,
		Subject: pkix.Name{CommonName: "leaf"},
		Issuer:  pkix.Name{CommonName: "root"},
	}
	root := &x509.Certificate{
		Raw:     []byte("root"),
		IsCA:    true,
		Subject: pkix.Name{CommonName: "root"},
		Issuer:  pkix.Name{CommonName: "root"},
	}

	if _, err := selectTopIntermediateCACert([]*x509.Certificate{leaf, root}); !errors.Is(err, errOIDCThumbprintUnverified) {
		t.Fatalf("selectTopIntermediateCACert() error = %v, want sentinel %v", err, errOIDCThumbprintUnverified)
	}
}

func TestSelectLongestVerifiedChain_ReturnsLongest(t *testing.T) {
	t.Parallel()

	short := []*x509.Certificate{{Raw: []byte("a")}}
	long := []*x509.Certificate{{Raw: []byte("a")}, {Raw: []byte("b")}, {Raw: []byte("c")}}

	got := selectLongestVerifiedChain([][]*x509.Certificate{short, long})
	if len(got) != len(long) {
		t.Fatalf("selectLongestVerifiedChain() len = %d, want %d", len(got), len(long))
	}
}

func TestEKSKarpenterBootstrapperReconciler_ResolveNodePoolTemplateYAML_UsesConfigMap(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testClusterName,
			Namespace: "default",
			Annotations: map[string]string{
				karpenterNodePoolTemplateConfigMapAnnotation:    "demo-template",
				karpenterNodePoolTemplateConfigMapKeyAnnotation: "custom.yaml",
			},
		},
	}
	templateCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "demo-template", Namespace: "default"},
		Data:       map[string]string{"custom.yaml": "kind: NodePool\nmetadata:\n  name: custom\n"},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, templateCM).Build()

	r := &EKSKarpenterBootstrapperReconciler{Client: c, Scheme: scheme}
	got, err := r.resolveNodePoolTemplateYAML(context.Background(), cluster, "eks-demo", "demo-profile", []string{"subnet-a", "subnet-b"}, []string{"sg-a"})
	if err != nil {
		t.Fatalf("resolveNodePoolTemplateYAML() error = %v", err)
	}
	if want := "kind: NodePool\nmetadata:\n  name: custom\n"; got != want {
		t.Fatalf("resolveNodePoolTemplateYAML() = %q, want %q", got, want)
	}
}

func TestEKSKarpenterBootstrapperReconciler_SuspendFluxKarpenterOnDelete(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	utilruntime.Must(clusterv1.AddToScheme(scheme))

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testClusterName,
			Namespace: "default",
			UID:       "cluster-uid",
		},
	}

	oci := &unstructured.Unstructured{}
	oci.SetGroupVersionKind(fluxOCIRepositoryGVK)
	oci.SetNamespace("default")
	oci.SetName("demo-karpenter")
	setManagedBy(oci)
	setClusterLabel(oci, testClusterName)
	mustSetNestedField(oci, "10m", "spec", "interval")

	hr := &unstructured.Unstructured{}
	hr.SetGroupVersionKind(fluxHelmReleaseGVK)
	hr.SetNamespace("default")
	hr.SetName("demo-karpenter")
	setManagedBy(hr)
	setClusterLabel(hr, testClusterName)
	mustSetNestedField(hr, "5m", "spec", "interval")

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, oci, hr).Build()
	r := &EKSKarpenterBootstrapperReconciler{Client: c, Scheme: scheme}

	changed, err := r.suspendFluxKarpenterOnDelete(context.Background(), cluster, testClusterName)
	if err != nil {
		t.Fatalf("suspendFluxKarpenterOnDelete() error = %v", err)
	}
	if !changed {
		t.Fatalf("suspendFluxKarpenterOnDelete() changed = false, want true")
	}

	gotOCI := getUnstructured(t, c, fluxOCIRepositoryGVK, "demo-karpenter")
	if suspended, found, err := unstructured.NestedBool(gotOCI.Object, "spec", "suspend"); err != nil {
		t.Fatalf("OCIRepository spec.suspend: %v", err)
	} else if !found || !suspended {
		t.Fatalf("OCIRepository spec.suspend = %v (found=%v), want true", suspended, found)
	}

	gotHR := getUnstructured(t, c, fluxHelmReleaseGVK, "demo-karpenter")
	if suspended, found, err := unstructured.NestedBool(gotHR.Object, "spec", "suspend"); err != nil {
		t.Fatalf("HelmRelease spec.suspend: %v", err)
	} else if !found || !suspended {
		t.Fatalf("HelmRelease spec.suspend = %v (found=%v), want true", suspended, found)
	}

	changedAgain, err := r.suspendFluxKarpenterOnDelete(context.Background(), cluster, testClusterName)
	if err != nil {
		t.Fatalf("second suspendFluxKarpenterOnDelete() error = %v", err)
	}
	if changedAgain {
		t.Fatalf("second suspendFluxKarpenterOnDelete() changed = true, want false")
	}
}

func getUnstructured(t *testing.T, c client.Client, gvk schema.GroupVersionKind, name string) *unstructured.Unstructured {
	t.Helper()
	namespace := "default"
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: name}, obj); err != nil {
		t.Fatalf("get %s %s/%s: %v", gvk.String(), namespace, name, err)
	}
	return obj
}

func findTagValue(t *testing.T, tags []any, key string) string {
	t.Helper()
	for _, item := range tags {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if gotKey, _ := m["key"].(string); gotKey == key {
			v, _ := m["value"].(string)
			return v
		}
	}
	return ""
}
