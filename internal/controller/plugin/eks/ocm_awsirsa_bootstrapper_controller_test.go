package eks

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	coreeks "github.com/reoring/kany8s/internal/plugin/eks"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestOCMAWSIRSASuffixMatchesOCMFormula(t *testing.T) {
	t.Parallel()

	got := ocmAWSIRSASuffix("147303435971", "pmc-next", "147303435971", "wlc-next")
	if want := "5354541d1e054ea41e036d2be9239892"; got != want {
		t.Fatalf("suffix = %q, want %q", got, want)
	}
}

func TestEKSOcmAWSIRSABootstrapReconciler_EnsureRoleCreatesExpectedSpec(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	utilruntime.Must(clusterv1.AddToScheme(scheme))

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wlc-next",
			Namespace: "wlc-next",
			Annotations: map[string]string{
				ocmHubClusterARNAnnotation: "arn:aws:eks:ap-northeast-1:147303435971:cluster/pmc-next",
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
	r := &EKSOcmAWSIRSABootstrapReconciler{Client: c, Scheme: scheme}

	hub, err := parseEKSClusterARN("arn:aws:eks:ap-northeast-1:147303435971:cluster/pmc-next")
	if err != nil {
		t.Fatalf("parse hub ARN: %v", err)
	}
	managed, err := parseEKSClusterARN("arn:aws:eks:ap-northeast-1:147303435971:cluster/wlc-next")
	if err != nil {
		t.Fatalf("parse managed ARN: %v", err)
	}
	suffix := ocmAWSIRSASuffix(hub.AccountID, hub.ClusterName, managed.AccountID, managed.ClusterName)
	roleName := ocmManagedClusterRolePrefix + suffix
	issuerHostPath := "oidc.eks.ap-northeast-1.amazonaws.com/id/CCA4AB0E737F3E51F0C7EB9F3B6986D2"
	oidcProviderARN := "arn:aws:iam::147303435971:oidc-provider/" + issuerHostPath
	hubRoleARN := "arn:aws:iam::147303435971:role/" + ocmHubRolePrefix + suffix

	assumePolicy, err := buildOCMManagedClusterAssumeRolePolicyDocument(oidcProviderARN, issuerHostPath)
	if err != nil {
		t.Fatalf("build trust policy: %v", err)
	}
	inlinePolicy, err := buildOCMManagedClusterAssumeHubPolicyDocument(hubRoleARN)
	if err != nil {
		t.Fatalf("build inline policy: %v", err)
	}

	ok, err := r.ensureOCMManagedClusterRole(
		context.Background(),
		cluster,
		roleName,
		hub.Region,
		roleName,
		assumePolicy,
		map[string]string{ocmManagedClusterAssumeHubName: inlinePolicy},
		ocmAWSIRSAResourceTags(cluster, hub, managed),
	)
	if err != nil {
		t.Fatalf("ensureOCMManagedClusterRole() error = %v", err)
	}
	if !ok {
		t.Fatalf("ensureOCMManagedClusterRole() ok = false, want true")
	}

	role := &unstructured.Unstructured{}
	role.SetGroupVersionKind(ackIAMRoleGVK)
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: cluster.Namespace, Name: roleName}, role); err != nil {
		t.Fatalf("get generated role: %v", err)
	}
	if got := strings.TrimSpace(role.GetAnnotations()[coreeks.ManagedByAnnotationKey]); got != ocmAWSIRSAManagedByValue {
		t.Fatalf("managed-by annotation = %q, want %q", got, ocmAWSIRSAManagedByValue)
	}
	if got, _, _ := unstructured.NestedString(role.Object, "spec", "name"); got != roleName {
		t.Fatalf("spec.name = %q, want %q", got, roleName)
	}
	if got, _, _ := unstructured.NestedString(role.Object, "spec", "assumeRolePolicyDocument"); !strings.Contains(got, oidcProviderARN) {
		t.Fatalf("trust policy does not contain OIDC provider ARN: %s", got)
	}
	if got, _, _ := unstructured.NestedString(role.Object, "spec", "assumeRolePolicyDocument"); !strings.Contains(got, "klusterlet-registration-sa") || !strings.Contains(got, "klusterlet-work-sa") {
		t.Fatalf("trust policy does not include both OCM service accounts: %s", got)
	}
	inline, found, err := unstructured.NestedStringMap(role.Object, "spec", "inlinePolicies")
	if err != nil {
		t.Fatalf("read inline policies: %v", err)
	}
	if !found {
		t.Fatalf("spec.inlinePolicies missing")
	}
	if got := inline[ocmManagedClusterAssumeHubName]; !strings.Contains(got, hubRoleARN) {
		t.Fatalf("assume-hub inline policy = %s, want resource %s", got, hubRoleARN)
	}

	tags, found, err := unstructured.NestedSlice(role.Object, "spec", "tags")
	if err != nil {
		t.Fatalf("read tags: %v", err)
	}
	if !found {
		t.Fatalf("spec.tags missing")
	}
	tagMap := map[string]string{}
	for _, raw := range tags {
		item, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("tag has unexpected shape: %#v", raw)
		}
		key, _ := item["key"].(string)
		value, _ := item["value"].(string)
		tagMap[key] = value
	}
	for key, want := range map[string]string{
		"hub_cluster_account_id":      "147303435971",
		"hub_cluster_name":            "pmc-next",
		"managed_cluster_account_id":  "147303435971",
		"managed_cluster_name":        "wlc-next",
		"kany8s.io/managed-by":        ocmAWSIRSAManagedByValue,
		"kany8s.io/cluster-name":      "wlc-next",
		"kany8s.io/cluster-namespace": "wlc-next",
	} {
		if got := tagMap[key]; got != want {
			t.Fatalf("tag %q = %q, want %q", key, got, want)
		}
	}
}

func TestBuildOCMManagedClusterAssumeHubPolicyDocument(t *testing.T) {
	t.Parallel()

	const hubRoleARN = "arn:aws:iam::147303435971:role/ocm-hub-5354541d1e054ea41e036d2be9239892"
	doc, err := buildOCMManagedClusterAssumeHubPolicyDocument(hubRoleARN)
	if err != nil {
		t.Fatalf("build policy: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(doc), &parsed); err != nil {
		t.Fatalf("policy is not JSON: %v", err)
	}
	if !strings.Contains(doc, `"Action":"sts:AssumeRole"`) || !strings.Contains(doc, hubRoleARN) {
		t.Fatalf("unexpected assume-hub policy: %s", doc)
	}
}
