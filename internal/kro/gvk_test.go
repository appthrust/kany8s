package kro

import (
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResolveInstanceGVK(t *testing.T) {
	t.Parallel()

	rgdGVK := schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "ResourceGraphDefinition"}

	tests := []struct {
		name             string
		schemaAPIVersion string
		schemaKind       string
		wantGVK          schema.GroupVersionKind
	}{
		{
			name:             "version only apiVersion uses kro.run group",
			schemaAPIVersion: "v1alpha1",
			schemaKind:       "EKSControlPlane",
			wantGVK:          schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "EKSControlPlane"},
		},
		{
			name:             "group/version apiVersion is used as-is",
			schemaAPIVersion: "example.com/v1alpha1",
			schemaKind:       "ExampleControlPlane",
			wantGVK:          schema.GroupVersionKind{Group: "example.com", Version: "v1alpha1", Kind: "ExampleControlPlane"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			scheme := runtime.NewScheme()
			scheme.AddKnownTypeWithName(rgdGVK, &unstructured.Unstructured{})
			scheme.AddKnownTypeWithName(rgdGVK.GroupVersion().WithKind("ResourceGraphDefinitionList"), &unstructured.UnstructuredList{})

			rgdName := testRGDName
			rgd := &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": rgdGVK.GroupVersion().String(),
					"kind":       rgdGVK.Kind,
					"metadata": map[string]any{
						"name": rgdName,
					},
					"spec": map[string]any{
						"schema": map[string]any{
							"apiVersion": tt.schemaAPIVersion,
							"kind":       tt.schemaKind,
						},
					},
				},
			}
			rgd.SetGroupVersionKind(rgdGVK)

			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rgd).Build()

			got, err := ResolveInstanceGVK(context.Background(), c, rgdName)
			if err != nil {
				t.Fatalf("ResolveInstanceGVK returned error: %v", err)
			}
			if got != tt.wantGVK {
				t.Fatalf("ResolveInstanceGVK returned %v, want %v", got, tt.wantGVK)
			}
		})
	}
}

func TestResolveInstanceGVK_Errors(t *testing.T) {
	t.Parallel()

	rgdGVK := schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "ResourceGraphDefinition"}

	tests := []struct {
		name           string
		rgdName        string
		rgdObject      map[string]any
		wantErrContain string
	}{
		{
			name:           "blank rgd name",
			rgdName:        " ",
			wantErrContain: "rgd name is required",
		},
		{
			name:    "missing spec.schema.apiVersion",
			rgdName: testRGDName,
			rgdObject: map[string]any{
				"apiVersion": rgdGVK.GroupVersion().String(),
				"kind":       rgdGVK.Kind,
				"metadata": map[string]any{
					"name": testRGDName,
				},
				"spec": map[string]any{
					"schema": map[string]any{
						"kind": "EKSControlPlane",
					},
				},
			},
			wantErrContain: "missing spec.schema.apiVersion",
		},
		{
			name:    "missing spec.schema.kind",
			rgdName: testRGDName,
			rgdObject: map[string]any{
				"apiVersion": rgdGVK.GroupVersion().String(),
				"kind":       rgdGVK.Kind,
				"metadata": map[string]any{
					"name": testRGDName,
				},
				"spec": map[string]any{
					"schema": map[string]any{
						"apiVersion": "v1alpha1",
					},
				},
			},
			wantErrContain: "missing spec.schema.kind",
		},
		{
			name:    "invalid spec.schema.apiVersion",
			rgdName: testRGDName,
			rgdObject: map[string]any{
				"apiVersion": rgdGVK.GroupVersion().String(),
				"kind":       rgdGVK.Kind,
				"metadata": map[string]any{
					"name": testRGDName,
				},
				"spec": map[string]any{
					"schema": map[string]any{
						"apiVersion": "example.com/",
						"kind":       "ExampleControlPlane",
					},
				},
			},
			wantErrContain: "invalid ResourceGraphDefinition schema apiVersion",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			scheme := runtime.NewScheme()
			scheme.AddKnownTypeWithName(rgdGVK, &unstructured.Unstructured{})
			scheme.AddKnownTypeWithName(rgdGVK.GroupVersion().WithKind("ResourceGraphDefinitionList"), &unstructured.UnstructuredList{})

			c := fake.NewClientBuilder().WithScheme(scheme).Build()
			if tt.rgdObject != nil {
				rgd := &unstructured.Unstructured{Object: tt.rgdObject}
				rgd.SetGroupVersionKind(rgdGVK)
				c = fake.NewClientBuilder().WithScheme(scheme).WithObjects(rgd).Build()
			}

			_, err := ResolveInstanceGVK(context.Background(), c, tt.rgdName)
			if err == nil {
				t.Fatalf("ResolveInstanceGVK unexpectedly succeeded")
			}
			if !strings.Contains(err.Error(), tt.wantErrContain) {
				t.Fatalf("ResolveInstanceGVK returned error %q, want to contain %q", err.Error(), tt.wantErrContain)
			}
		})
	}
}
