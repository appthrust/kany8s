package controller

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"

	controlplanev1alpha1 "github.com/reoring/kany8s/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/controllers/external"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"
)

func TestClusterTopologyVersionChangePropagatesToKroInstance(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Topology controller requires CAPI contract labels on provider CRDs.
	crdPath := filepath.Join("..", "..", "config", "crd", "bases", "controlplane.cluster.x-k8s.io_kany8scontrolplanes.yaml")
	crdBytes, err := os.ReadFile(crdPath)
	g.Expect(err).NotTo(HaveOccurred())
	crd := &apiextensionsv1.CustomResourceDefinition{}
	g.Expect(yaml.Unmarshal(crdBytes, crd)).To(Succeed())
	g.Expect(crd.Labels).NotTo(BeNil())
	g.Expect(crd.Labels["cluster.x-k8s.io/v1beta2"]).To(Equal("v1alpha1"))

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(apiextensionsv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(controlplanev1alpha1.AddToScheme(scheme)).To(Succeed())

	rgdGVK := schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "ResourceGraphDefinition"}
	instanceGVK := schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "EKSControlPlane"}
	scheme.AddKnownTypeWithName(rgdGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(rgdGVK.GroupVersion().WithKind("ResourceGraphDefinitionList"), &unstructured.UnstructuredList{})
	scheme.AddKnownTypeWithName(instanceGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(instanceGVK.GroupVersion().WithKind("EKSControlPlaneList"), &unstructured.UnstructuredList{})

	const (
		clusterName      = "demo-cluster"
		clusterNamespace = "default"
		rgdName          = "eks-control-plane.kro.run"
		initialVersion   = "1.34"
		upgradedVersion  = "1.35"
	)

	cpTemplate := &controlplanev1alpha1.Kany8sControlPlaneTemplate{
		TypeMeta: metav1.TypeMeta{APIVersion: controlplanev1alpha1.GroupVersion.String(), Kind: "Kany8sControlPlaneTemplate"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kany8s-eks",
			Namespace: clusterNamespace,
		},
		Spec: controlplanev1alpha1.Kany8sControlPlaneTemplateSpec{
			Template: controlplanev1alpha1.Kany8sControlPlaneTemplateResource{
				Spec: controlplanev1alpha1.Kany8sControlPlaneTemplateResourceSpec{
					ResourceGraphDefinitionRef: controlplanev1alpha1.ResourceGraphDefinitionReference{Name: rgdName},
				},
			},
		},
	}

	cpTemplateUnstructured := mustToUnstructured(t, cpTemplate)

	generatedCP, err := external.GenerateTemplate(&external.GenerateTemplateInput{
		Template: cpTemplateUnstructured,
		TemplateRef: &corev1.ObjectReference{
			APIVersion: cpTemplate.APIVersion,
			Kind:       cpTemplate.Kind,
			Namespace:  cpTemplate.Namespace,
			Name:       cpTemplate.Name,
		},
		Namespace:   clusterNamespace,
		Name:        clusterName,
		ClusterName: clusterName,
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(unstructured.SetNestedField(generatedCP.Object, initialVersion, "spec", "version")).To(Succeed())

	cp := &controlplanev1alpha1.Kany8sControlPlane{}
	g.Expect(runtime.DefaultUnstructuredConverter.FromUnstructured(generatedCP.Object, cp)).To(Succeed())
	// Ensure TypeMeta is set; converter relies on apiVersion/kind fields.
	cp.APIVersion = controlplanev1alpha1.GroupVersion.String()
	cp.Kind = kany8sControlPlaneKind

	g.Expect(cp.Name).To(Equal(clusterName))
	g.Expect(cp.Namespace).To(Equal(clusterNamespace))
	g.Expect(cp.Spec.Version).To(Equal(initialVersion))
	g.Expect(cp.Spec.ResourceGraphDefinitionRef.Name).To(Equal(rgdName))

	rgd := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": rgdGVK.GroupVersion().String(),
		"kind":       rgdGVK.Kind,
		"metadata": map[string]any{
			"name": rgdName,
		},
		"spec": map[string]any{
			"schema": map[string]any{
				"apiVersion": "v1alpha1",
				"kind":       instanceGVK.Kind,
			},
		},
	}}
	rgd.SetGroupVersionKind(rgdGVK)

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cp, rgd).
		WithStatusSubresource(cp).
		Build()

	reconciler := &Kany8sControlPlaneReconciler{Client: c, Scheme: scheme}
	_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: clusterName, Namespace: clusterNamespace}})
	g.Expect(err).NotTo(HaveOccurred())

	instance := &unstructured.Unstructured{}
	instance.SetGroupVersionKind(instanceGVK)
	g.Expect(c.Get(ctx, client.ObjectKey{Name: clusterName, Namespace: clusterNamespace}, instance)).To(Succeed())

	instanceVersion, found, err := unstructured.NestedString(instance.Object, "spec", "version")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(instanceVersion).To(Equal(initialVersion))

	// Before Ready, the control plane should not report status.version.
	gotCP := &controlplanev1alpha1.Kany8sControlPlane{}
	g.Expect(c.Get(ctx, client.ObjectKey{Name: clusterName, Namespace: clusterNamespace}, gotCP)).To(Succeed())
	statusVersionBefore, _, err := unstructured.NestedString(mustToUnstructured(t, gotCP).Object, "status", "version")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(statusVersionBefore).To(BeEmpty())

	// Mark the kro instance Ready so the controller can finalize status fields.
	g.Expect(unstructured.SetNestedField(instance.Object, true, "status", "ready")).To(Succeed())
	g.Expect(unstructured.SetNestedField(instance.Object, "https://api.demo.example.com:6443", "status", "endpoint")).To(Succeed())
	g.Expect(c.Update(ctx, instance)).To(Succeed())

	_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: clusterName, Namespace: clusterNamespace}})
	g.Expect(err).NotTo(HaveOccurred())

	gotCP = &controlplanev1alpha1.Kany8sControlPlane{}
	g.Expect(c.Get(ctx, client.ObjectKey{Name: clusterName, Namespace: clusterNamespace}, gotCP)).To(Succeed())
	gotCPUnstructured := mustToUnstructured(t, gotCP)
	gotCPUnstructured.SetGroupVersionKind(schema.GroupVersionKind{Group: controlplanev1alpha1.GroupVersion.Group, Version: controlplanev1alpha1.GroupVersion.Version, Kind: kany8sControlPlaneKind})

	statusVersionAfter, statusVersionFound, err := unstructured.NestedString(gotCPUnstructured.Object, "status", "version")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(statusVersionFound).To(BeTrue())
	g.Expect(statusVersionAfter).To(Equal(initialVersion))

	// Simulate topology controller updating the control plane version.
	gotCP.Spec.Version = upgradedVersion
	g.Expect(c.Update(ctx, gotCP)).To(Succeed())

	_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: clusterName, Namespace: clusterNamespace}})
	g.Expect(err).NotTo(HaveOccurred())

	gotInstance := &unstructured.Unstructured{}
	gotInstance.SetGroupVersionKind(instanceGVK)
	g.Expect(c.Get(ctx, client.ObjectKey{Name: clusterName, Namespace: clusterNamespace}, gotInstance)).To(Succeed())
	gotInstanceVersion, found, err := unstructured.NestedString(gotInstance.Object, "spec", "version")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(gotInstanceVersion).To(Equal(upgradedVersion))
}

func mustToUnstructured(t *testing.T, obj runtime.Object) *unstructured.Unstructured {
	t.Helper()

	u := &unstructured.Unstructured{}
	fields, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		t.Fatalf("convert to unstructured: %v", err)
	}
	u.Object = fields
	return u
}
