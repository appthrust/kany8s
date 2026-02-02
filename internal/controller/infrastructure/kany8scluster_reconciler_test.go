package infrastructure

import (
	"context"
	"testing"

	infrastructurev1alpha1 "github.com/reoring/kany8s/api/infrastructure/v1alpha1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testClusterName = "demo"
	testNamespace   = "default"
	testRGDName     = "demo-infra"
	testRegion      = "us-west-2"
	testTagEnv      = "dev"
	testKroSpecRaw  = `{"region":"us-west-2","tags":{"env":"dev"}}`
)

func TestKany8sClusterReconciler_SetsReadyConditionTrue(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := infrastructurev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Kany8sCluster scheme: %v", err)
	}

	kc := &infrastructurev1alpha1.Kany8sCluster{ObjectMeta: metav1.ObjectMeta{Name: testClusterName, Namespace: testNamespace}}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kc).WithStatusSubresource(kc).Build()
	r := &Kany8sClusterReconciler{Client: c, Scheme: scheme}

	ctx := context.Background()
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: testClusterName, Namespace: testNamespace}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got := &infrastructurev1alpha1.Kany8sCluster{}
	if err := c.Get(ctx, client.ObjectKey{Name: testClusterName, Namespace: testNamespace}, got); err != nil {
		t.Fatalf("get Kany8sCluster: %v", err)
	}

	cond := meta.FindStatusCondition(got.Status.Conditions, "Ready")
	if cond == nil {
		t.Fatalf("expected Ready condition")
	}
	if cond.Status != metav1.ConditionTrue {
		t.Fatalf("Ready condition status = %q, want %q", cond.Status, metav1.ConditionTrue)
	}
}

func TestKany8sClusterReconciler_SetsInitializationProvisionedTrue(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := infrastructurev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Kany8sCluster scheme: %v", err)
	}

	kc := &infrastructurev1alpha1.Kany8sCluster{ObjectMeta: metav1.ObjectMeta{Name: testClusterName, Namespace: testNamespace}}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kc).WithStatusSubresource(kc).Build()
	r := &Kany8sClusterReconciler{Client: c, Scheme: scheme}

	ctx := context.Background()
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: testClusterName, Namespace: testNamespace}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got := &infrastructurev1alpha1.Kany8sCluster{}
	if err := c.Get(ctx, client.ObjectKey{Name: testClusterName, Namespace: testNamespace}, got); err != nil {
		t.Fatalf("get Kany8sCluster: %v", err)
	}

	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(got)
	if err != nil {
		t.Fatalf("to unstructured: %v", err)
	}

	provisioned, found, err := unstructured.NestedBool(u, "status", "initialization", "provisioned")
	if err != nil {
		t.Fatalf("get status.initialization.provisioned: %v", err)
	}
	if !found {
		t.Fatalf("expected status.initialization.provisioned to be set")
	}
	if !provisioned {
		t.Fatalf("status.initialization.provisioned = false, want true")
	}
}

func TestKany8sClusterReconciler_CreatesKroInstanceWhenResourceGraphDefinitionRefIsSet(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := infrastructurev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Kany8sCluster scheme: %v", err)
	}

	rgdGVK := schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "ResourceGraphDefinition"}
	instanceGVK := schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "DemoInfra"}

	scheme.AddKnownTypeWithName(rgdGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(rgdGVK.GroupVersion().WithKind("ResourceGraphDefinitionList"), &unstructured.UnstructuredList{})
	scheme.AddKnownTypeWithName(instanceGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(instanceGVK.GroupVersion().WithKind("DemoInfraList"), &unstructured.UnstructuredList{})

	kc := &infrastructurev1alpha1.Kany8sCluster{
		ObjectMeta: metav1.ObjectMeta{Name: testClusterName, Namespace: testNamespace},
		Spec: infrastructurev1alpha1.Kany8sClusterSpec{
			ResourceGraphDefinitionRef: &infrastructurev1alpha1.ResourceGraphDefinitionReference{Name: testRGDName},
		},
	}

	rgd := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": rgdGVK.GroupVersion().String(),
		"kind":       rgdGVK.Kind,
		"metadata": map[string]any{
			"name": testRGDName,
		},
		"spec": map[string]any{
			"schema": map[string]any{
				"apiVersion": "v1alpha1",
				"kind":       instanceGVK.Kind,
			},
		},
	}}
	rgd.SetGroupVersionKind(rgdGVK)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kc, rgd).WithStatusSubresource(kc).Build()
	r := &Kany8sClusterReconciler{Client: c, Scheme: scheme}

	ctx := context.Background()
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: testClusterName, Namespace: testNamespace}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got := &unstructured.Unstructured{}
	got.SetGroupVersionKind(instanceGVK)
	if err := c.Get(ctx, client.ObjectKey{Name: testClusterName, Namespace: testNamespace}, got); err != nil {
		t.Fatalf("get kro instance: %v", err)
	}
	if got.GetName() != testClusterName {
		t.Fatalf("kro instance name = %q, want %q", got.GetName(), testClusterName)
	}
	if got.GetNamespace() != testNamespace {
		t.Fatalf("kro instance namespace = %q, want %q", got.GetNamespace(), testNamespace)
	}
}

func TestKany8sClusterReconciler_RendersKroInstanceSpecFromKroSpecAndClusterMetadata(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := infrastructurev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Kany8sCluster scheme: %v", err)
	}

	rgdGVK := schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "ResourceGraphDefinition"}
	instanceGVK := schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "DemoInfra"}

	scheme.AddKnownTypeWithName(rgdGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(rgdGVK.GroupVersion().WithKind("ResourceGraphDefinitionList"), &unstructured.UnstructuredList{})
	scheme.AddKnownTypeWithName(instanceGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(instanceGVK.GroupVersion().WithKind("DemoInfraList"), &unstructured.UnstructuredList{})

	kc := &infrastructurev1alpha1.Kany8sCluster{
		ObjectMeta: metav1.ObjectMeta{Name: testClusterName, Namespace: testNamespace},
		Spec: infrastructurev1alpha1.Kany8sClusterSpec{
			ResourceGraphDefinitionRef: &infrastructurev1alpha1.ResourceGraphDefinitionReference{Name: testRGDName},
			KroSpec:                    &apiextensionsv1.JSON{Raw: []byte(testKroSpecRaw)},
		},
	}

	rgd := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": rgdGVK.GroupVersion().String(),
		"kind":       rgdGVK.Kind,
		"metadata": map[string]any{
			"name": testRGDName,
		},
		"spec": map[string]any{
			"schema": map[string]any{
				"apiVersion": "v1alpha1",
				"kind":       instanceGVK.Kind,
			},
		},
	}}
	rgd.SetGroupVersionKind(rgdGVK)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kc, rgd).WithStatusSubresource(kc).Build()
	r := &Kany8sClusterReconciler{Client: c, Scheme: scheme}

	ctx := context.Background()
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: testClusterName, Namespace: testNamespace}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got := &unstructured.Unstructured{}
	got.SetGroupVersionKind(instanceGVK)
	if err := c.Get(ctx, client.ObjectKey{Name: testClusterName, Namespace: testNamespace}, got); err != nil {
		t.Fatalf("get kro instance: %v", err)
	}

	clusterName, found, err := unstructured.NestedString(got.Object, "spec", "clusterName")
	if err != nil {
		t.Fatalf("get spec.clusterName: %v", err)
	}
	if !found {
		t.Fatalf("expected spec.clusterName to be set")
	}
	if clusterName != testClusterName {
		t.Fatalf("spec.clusterName = %q, want %q", clusterName, testClusterName)
	}

	clusterNamespace, found, err := unstructured.NestedString(got.Object, "spec", "clusterNamespace")
	if err != nil {
		t.Fatalf("get spec.clusterNamespace: %v", err)
	}
	if !found {
		t.Fatalf("expected spec.clusterNamespace to be set")
	}
	if clusterNamespace != testNamespace {
		t.Fatalf("spec.clusterNamespace = %q, want %q", clusterNamespace, testNamespace)
	}

	region, found, err := unstructured.NestedString(got.Object, "spec", "region")
	if err != nil {
		t.Fatalf("get spec.region: %v", err)
	}
	if !found {
		t.Fatalf("expected spec.region to be set")
	}
	if region != testRegion {
		t.Fatalf("spec.region = %q, want %q", region, testRegion)
	}

	env, found, err := unstructured.NestedString(got.Object, "spec", "tags", "env")
	if err != nil {
		t.Fatalf("get spec.tags.env: %v", err)
	}
	if !found {
		t.Fatalf("expected spec.tags.env to be set")
	}
	if env != testTagEnv {
		t.Fatalf("spec.tags.env = %q, want %q", env, testTagEnv)
	}
}

func TestKany8sClusterReconciler_RevertsKroInstanceSpecDrift(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := infrastructurev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Kany8sCluster scheme: %v", err)
	}

	rgdGVK := schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "ResourceGraphDefinition"}
	instanceGVK := schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "DemoInfra"}

	scheme.AddKnownTypeWithName(rgdGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(rgdGVK.GroupVersion().WithKind("ResourceGraphDefinitionList"), &unstructured.UnstructuredList{})
	scheme.AddKnownTypeWithName(instanceGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(instanceGVK.GroupVersion().WithKind("DemoInfraList"), &unstructured.UnstructuredList{})

	kc := &infrastructurev1alpha1.Kany8sCluster{
		ObjectMeta: metav1.ObjectMeta{Name: testClusterName, Namespace: testNamespace},
		Spec: infrastructurev1alpha1.Kany8sClusterSpec{
			ResourceGraphDefinitionRef: &infrastructurev1alpha1.ResourceGraphDefinitionReference{Name: testRGDName},
			KroSpec:                    &apiextensionsv1.JSON{Raw: []byte(testKroSpecRaw)},
		},
	}

	rgd := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": rgdGVK.GroupVersion().String(),
		"kind":       rgdGVK.Kind,
		"metadata": map[string]any{
			"name": testRGDName,
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
			"name":      testClusterName,
			"namespace": testNamespace,
		},
		"spec": map[string]any{
			"clusterName":      "drifted",
			"clusterNamespace": "other",
			"region":           "eu-west-1",
			"tags": map[string]any{
				"env": "prod",
			},
		},
	}}
	instance.SetGroupVersionKind(instanceGVK)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kc, rgd, instance).WithStatusSubresource(kc).Build()
	r := &Kany8sClusterReconciler{Client: c, Scheme: scheme}

	ctx := context.Background()
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: testClusterName, Namespace: testNamespace}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got := &unstructured.Unstructured{}
	got.SetGroupVersionKind(instanceGVK)
	if err := c.Get(ctx, client.ObjectKey{Name: testClusterName, Namespace: testNamespace}, got); err != nil {
		t.Fatalf("get kro instance: %v", err)
	}

	clusterName, found, err := unstructured.NestedString(got.Object, "spec", "clusterName")
	if err != nil {
		t.Fatalf("get spec.clusterName: %v", err)
	}
	if !found {
		t.Fatalf("expected spec.clusterName to be set")
	}
	if clusterName != testClusterName {
		t.Fatalf("spec.clusterName = %q, want %q", clusterName, testClusterName)
	}

	clusterNamespace, found, err := unstructured.NestedString(got.Object, "spec", "clusterNamespace")
	if err != nil {
		t.Fatalf("get spec.clusterNamespace: %v", err)
	}
	if !found {
		t.Fatalf("expected spec.clusterNamespace to be set")
	}
	if clusterNamespace != testNamespace {
		t.Fatalf("spec.clusterNamespace = %q, want %q", clusterNamespace, testNamespace)
	}

	region, found, err := unstructured.NestedString(got.Object, "spec", "region")
	if err != nil {
		t.Fatalf("get spec.region: %v", err)
	}
	if !found {
		t.Fatalf("expected spec.region to be set")
	}
	if region != testRegion {
		t.Fatalf("spec.region = %q, want %q", region, testRegion)
	}

	env, found, err := unstructured.NestedString(got.Object, "spec", "tags", "env")
	if err != nil {
		t.Fatalf("get spec.tags.env: %v", err)
	}
	if !found {
		t.Fatalf("expected spec.tags.env to be set")
	}
	if env != testTagEnv {
		t.Fatalf("spec.tags.env = %q, want %q", env, testTagEnv)
	}
}

func TestKany8sClusterReconciler_KroMode_WaitsUntilInstanceReady(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := infrastructurev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Kany8sCluster scheme: %v", err)
	}

	rgdGVK := schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "ResourceGraphDefinition"}
	instanceGVK := schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "DemoInfra"}

	scheme.AddKnownTypeWithName(rgdGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(rgdGVK.GroupVersion().WithKind("ResourceGraphDefinitionList"), &unstructured.UnstructuredList{})
	scheme.AddKnownTypeWithName(instanceGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(instanceGVK.GroupVersion().WithKind("DemoInfraList"), &unstructured.UnstructuredList{})

	kc := &infrastructurev1alpha1.Kany8sCluster{
		ObjectMeta: metav1.ObjectMeta{Name: testClusterName, Namespace: testNamespace},
		Spec: infrastructurev1alpha1.Kany8sClusterSpec{
			ResourceGraphDefinitionRef: &infrastructurev1alpha1.ResourceGraphDefinitionReference{Name: testRGDName},
		},
	}

	rgd := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": rgdGVK.GroupVersion().String(),
		"kind":       rgdGVK.Kind,
		"metadata": map[string]any{
			"name": testRGDName,
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
			"name":      testClusterName,
			"namespace": testNamespace,
		},
		"status": map[string]any{
			"ready":   false,
			"reason":  "Provisioning",
			"message": "waiting for infrastructure",
		},
	}}
	instance.SetGroupVersionKind(instanceGVK)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kc, rgd, instance).WithStatusSubresource(kc).Build()
	r := &Kany8sClusterReconciler{Client: c, Scheme: scheme}

	ctx := context.Background()
	res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: testClusterName, Namespace: testNamespace}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.RequeueAfter == 0 {
		t.Fatalf("expected RequeueAfter to be set while waiting")
	}

	got := &infrastructurev1alpha1.Kany8sCluster{}
	if err := c.Get(ctx, client.ObjectKey{Name: testClusterName, Namespace: testNamespace}, got); err != nil {
		t.Fatalf("get Kany8sCluster: %v", err)
	}

	if got.Status.Initialization.Provisioned {
		t.Fatalf("status.initialization.provisioned = true, want false")
	}
	cond := meta.FindStatusCondition(got.Status.Conditions, "Ready")
	if cond == nil {
		t.Fatalf("expected Ready condition")
	}
	if cond.Status != metav1.ConditionFalse {
		t.Fatalf("Ready condition status = %q, want %q", cond.Status, metav1.ConditionFalse)
	}
	if got.Status.FailureReason != nil {
		t.Fatalf("status.failureReason = %q, want nil", *got.Status.FailureReason)
	}
	if got.Status.FailureMessage != nil {
		t.Fatalf("status.failureMessage = %q, want nil", *got.Status.FailureMessage)
	}
}

func TestKany8sClusterReconciler_KroMode_SetsProvisionedWhenInstanceReady(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := infrastructurev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Kany8sCluster scheme: %v", err)
	}

	rgdGVK := schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "ResourceGraphDefinition"}
	instanceGVK := schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "DemoInfra"}

	scheme.AddKnownTypeWithName(rgdGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(rgdGVK.GroupVersion().WithKind("ResourceGraphDefinitionList"), &unstructured.UnstructuredList{})
	scheme.AddKnownTypeWithName(instanceGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(instanceGVK.GroupVersion().WithKind("DemoInfraList"), &unstructured.UnstructuredList{})

	kc := &infrastructurev1alpha1.Kany8sCluster{
		ObjectMeta: metav1.ObjectMeta{Name: testClusterName, Namespace: testNamespace},
		Spec: infrastructurev1alpha1.Kany8sClusterSpec{
			ResourceGraphDefinitionRef: &infrastructurev1alpha1.ResourceGraphDefinitionReference{Name: testRGDName},
		},
	}

	rgd := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": rgdGVK.GroupVersion().String(),
		"kind":       rgdGVK.Kind,
		"metadata": map[string]any{
			"name": testRGDName,
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
			"name":      testClusterName,
			"namespace": testNamespace,
		},
		"status": map[string]any{
			"ready":   true,
			"reason":  "Ready",
			"message": "infrastructure is ready",
		},
	}}
	instance.SetGroupVersionKind(instanceGVK)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(kc, rgd, instance).WithStatusSubresource(kc).Build()
	r := &Kany8sClusterReconciler{Client: c, Scheme: scheme}

	ctx := context.Background()
	res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: testClusterName, Namespace: testNamespace}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.RequeueAfter != 0 {
		t.Fatalf("expected no RequeueAfter when provisioned")
	}

	got := &infrastructurev1alpha1.Kany8sCluster{}
	if err := c.Get(ctx, client.ObjectKey{Name: testClusterName, Namespace: testNamespace}, got); err != nil {
		t.Fatalf("get Kany8sCluster: %v", err)
	}

	if !got.Status.Initialization.Provisioned {
		t.Fatalf("status.initialization.provisioned = false, want true")
	}
	cond := meta.FindStatusCondition(got.Status.Conditions, "Ready")
	if cond == nil {
		t.Fatalf("expected Ready condition")
	}
	if cond.Status != metav1.ConditionTrue {
		t.Fatalf("Ready condition status = %q, want %q", cond.Status, metav1.ConditionTrue)
	}
}
