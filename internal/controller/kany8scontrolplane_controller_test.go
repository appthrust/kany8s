/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	controlplanev1alpha1 "github.com/reoring/kany8s/api/v1alpha1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apixclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var _ = Describe("Kany8sControlPlane Controller", func() {
	Context("When reconciling a resource", func() {
		It("should enqueue a reconcile when the kro instance is updated", func() {
			const (
				demoEndpointHost = "api.demo.example.com"
				demoEndpointURL  = "https://" + demoEndpointHost + ":6443"
			)

			ctx, cancel := context.WithCancel(context.Background())
			DeferCleanup(cancel)

			apiExt, err := apixclientset.NewForConfig(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(ensureCRDEstablished(ctx, apiExt, kroResourceGraphDefinitionCRD())).To(Succeed())
			Expect(ensureCRDEstablished(ctx, apiExt, kroInstanceCRD())).To(Succeed())

			scheme := runtime.NewScheme()
			Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
			Expect(controlplanev1alpha1.AddToScheme(scheme)).To(Succeed())

			mgr, err := ctrl.NewManager(cfg, ctrl.Options{
				Scheme:                 scheme,
				Metrics:                metricsserver.Options{BindAddress: "0"},
				HealthProbeBindAddress: "0",
				LeaderElection:         false,
			})
			Expect(err).NotTo(HaveOccurred())

			r := &Kany8sControlPlaneReconciler{
				Client:   mgr.GetClient(),
				Scheme:   mgr.GetScheme(),
				Recorder: mgr.GetEventRecorderFor("kany8scontrolplane"), //nolint:staticcheck
			}
			Expect(r.SetupWithManager(mgr)).To(Succeed())

			go func() {
				defer GinkgoRecover()
				Expect(mgr.Start(ctx)).To(Succeed())
			}()

			c, err := client.New(cfg, client.Options{Scheme: scheme})
			Expect(err).NotTo(HaveOccurred())

			rgdGVK := schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "ResourceGraphDefinition"}
			instanceGVK := schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "EKSControlPlane"}

			rgd := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": rgdGVK.GroupVersion().String(),
				"kind":       rgdGVK.Kind,
				"metadata": map[string]any{
					"name": "eks-control-plane",
				},
				"spec": map[string]any{
					"schema": map[string]any{
						"apiVersion": "v1alpha1",
						"kind":       instanceGVK.Kind,
					},
				},
			}}
			rgd.SetGroupVersionKind(rgdGVK)
			if err := c.Create(ctx, rgd); err != nil && !apierrors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			cpGVK := schema.GroupVersionKind{Group: "controlplane.cluster.x-k8s.io", Version: "v1alpha1", Kind: "Kany8sControlPlane"}
			cp := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": cpGVK.GroupVersion().String(),
				"kind":       cpGVK.Kind,
				"metadata": map[string]any{
					"name":      "demo",
					"namespace": "default",
				},
				"spec": map[string]any{
					"version": "1.34",
					"resourceGraphDefinitionRef": map[string]any{
						"name": "eks-control-plane",
					},
				},
			}}
			cp.SetGroupVersionKind(cpGVK)
			if err := c.Create(ctx, cp); err != nil && !apierrors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			Eventually(func() error {
				instance := &unstructured.Unstructured{}
				instance.SetGroupVersionKind(instanceGVK)
				return c.Get(ctx, client.ObjectKey{Name: "demo", Namespace: "default"}, instance)
			}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

			instance := &unstructured.Unstructured{}
			instance.SetGroupVersionKind(instanceGVK)
			Expect(c.Get(ctx, client.ObjectKey{Name: "demo", Namespace: "default"}, instance)).To(Succeed())
			Expect(unstructured.SetNestedField(instance.Object, true, "status", "ready")).To(Succeed())
			Expect(unstructured.SetNestedField(instance.Object, demoEndpointURL, "status", "endpoint")).To(Succeed())
			Expect(c.Update(ctx, instance)).To(Succeed())

			Eventually(func() error {
				got := &controlplanev1alpha1.Kany8sControlPlane{}
				if err := c.Get(ctx, client.ObjectKey{Name: "demo", Namespace: "default"}, got); err != nil {
					return err
				}
				if got.Spec.ControlPlaneEndpoint.Host != demoEndpointHost {
					return fmt.Errorf("endpoint host = %q, want %q", got.Spec.ControlPlaneEndpoint.Host, demoEndpointHost)
				}
				if got.Spec.ControlPlaneEndpoint.Port != 6443 {
					return fmt.Errorf("endpoint port = %d, want %d", got.Spec.ControlPlaneEndpoint.Port, 6443)
				}
				if !got.Status.Initialization.ControlPlaneInitialized {
					return fmt.Errorf("control plane initialized = false, want true")
				}
				return nil
			}, 3*time.Second, 100*time.Millisecond).Should(Succeed())
		})
	})
})

func ensureCRDEstablished(ctx context.Context, apiExt *apixclientset.Clientset, crd *apiextensionsv1.CustomResourceDefinition) error {
	_, err := apiExt.ApiextensionsV1().CustomResourceDefinitions().Create(ctx, crd, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("timed out waiting for CRD %q to be established", crd.Name)
		case <-ticker.C:
			got, err := apiExt.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crd.Name, metav1.GetOptions{})
			if err != nil {
				continue
			}
			for _, cond := range got.Status.Conditions {
				if cond.Type == apiextensionsv1.Established && cond.Status == apiextensionsv1.ConditionTrue {
					return nil
				}
			}
		}
	}
}

func kroResourceGraphDefinitionCRD() *apiextensionsv1.CustomResourceDefinition {
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "resourcegraphdefinitions.kro.run",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "kro.run",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   "resourcegraphdefinitions",
				Singular: "resourcegraphdefinition",
				Kind:     "ResourceGraphDefinition",
			},
			Scope: apiextensionsv1.ClusterScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1alpha1",
					Served:  true,
					Storage: true,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type:                   "object",
							XPreserveUnknownFields: boolPtr(true),
						},
					},
				},
			},
		},
	}
}

func kroInstanceCRD() *apiextensionsv1.CustomResourceDefinition {
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ekscontrolplanes.kro.run",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "kro.run",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   "ekscontrolplanes",
				Singular: "ekscontrolplane",
				Kind:     "EKSControlPlane",
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1alpha1",
					Served:  true,
					Storage: true,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type:                   "object",
							XPreserveUnknownFields: boolPtr(true),
						},
					},
				},
			},
		},
	}
}

func boolPtr(b bool) *bool {
	return &b
}
