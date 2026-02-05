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

func TestSchemaHasSpecField(t *testing.T) {
	t.Parallel()

	rgdGVK := schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "ResourceGraphDefinition"}

	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(rgdGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(rgdGVK.GroupVersion().WithKind("ResourceGraphDefinitionList"), &unstructured.UnstructuredList{})

	rgdName := testRGDName
	rgd := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": rgdGVK.GroupVersion().String(),
		"kind":       rgdGVK.Kind,
		"metadata": map[string]any{
			"name": rgdName,
		},
		"spec": map[string]any{
			"schema": map[string]any{
				"apiVersion": "v1alpha1",
				"kind":       "DemoInfrastructure",
				"spec": map[string]any{
					"clusterName":      "string",
					"clusterNamespace": "string",
					"clusterUID":       "string",
					"vpc": map[string]any{
						"subnetIDs": "[]string",
					},
				},
			},
		},
	}}
	rgd.SetGroupVersionKind(rgdGVK)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rgd).Build()

	got, err := SchemaHasSpecField(context.Background(), c, rgdName, "clusterUID")
	if err != nil {
		t.Fatalf("SchemaHasSpecField returned error: %v", err)
	}
	if !got {
		t.Fatalf("SchemaHasSpecField returned %v, want %v", got, true)
	}

	got, err = SchemaHasSpecField(context.Background(), c, rgdName, "missingField")
	if err != nil {
		t.Fatalf("SchemaHasSpecField returned error: %v", err)
	}
	if got {
		t.Fatalf("SchemaHasSpecField returned %v, want %v", got, false)
	}

	got, err = SchemaHasSpecField(context.Background(), c, rgdName, "vpc.subnetIDs")
	if err != nil {
		t.Fatalf("SchemaHasSpecField returned error: %v", err)
	}
	if !got {
		t.Fatalf("SchemaHasSpecField returned %v, want %v", got, true)
	}
}

func TestSchemaHasSpecField_MissingSchemaSpec(t *testing.T) {
	t.Parallel()

	rgdGVK := schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "ResourceGraphDefinition"}

	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(rgdGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(rgdGVK.GroupVersion().WithKind("ResourceGraphDefinitionList"), &unstructured.UnstructuredList{})

	rgdName := testRGDName
	rgd := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": rgdGVK.GroupVersion().String(),
		"kind":       rgdGVK.Kind,
		"metadata": map[string]any{
			"name": rgdName,
		},
		"spec": map[string]any{
			"schema": map[string]any{
				"apiVersion": "v1alpha1",
				"kind":       "DemoInfrastructure",
			},
		},
	}}
	rgd.SetGroupVersionKind(rgdGVK)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rgd).Build()

	got, err := SchemaHasSpecField(context.Background(), c, rgdName, "clusterUID")
	if err != nil {
		t.Fatalf("SchemaHasSpecField returned error: %v", err)
	}
	if got {
		t.Fatalf("SchemaHasSpecField returned %v, want %v", got, false)
	}
}

func TestSchemaHasSpecField_Errors(t *testing.T) {
	t.Parallel()

	rgdGVK := schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "ResourceGraphDefinition"}

	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(rgdGVK, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(rgdGVK.GroupVersion().WithKind("ResourceGraphDefinitionList"), &unstructured.UnstructuredList{})

	rgdName := testRGDName
	rgd := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": rgdGVK.GroupVersion().String(),
		"kind":       rgdGVK.Kind,
		"metadata": map[string]any{
			"name": rgdName,
		},
		"spec": map[string]any{
			"schema": map[string]any{
				"apiVersion": "v1alpha1",
				"kind":       "DemoInfrastructure",
				"spec":       "not-a-map",
			},
		},
	}}
	rgd.SetGroupVersionKind(rgdGVK)

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rgd).Build()

	if _, err := SchemaHasSpecField(context.Background(), c, " ", "clusterUID"); err == nil {
		t.Fatalf("SchemaHasSpecField unexpectedly succeeded")
	}

	if _, err := SchemaHasSpecField(context.Background(), c, rgdName, " "); err == nil {
		t.Fatalf("SchemaHasSpecField unexpectedly succeeded")
	}

	if _, err := SchemaHasSpecField(context.Background(), c, rgdName, "a..b"); err == nil {
		t.Fatalf("SchemaHasSpecField unexpectedly succeeded")
	}

	_, err := SchemaHasSpecField(context.Background(), c, rgdName, "clusterUID")
	if err == nil {
		t.Fatalf("SchemaHasSpecField unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "read ResourceGraphDefinition spec.schema.spec") {
		t.Fatalf("SchemaHasSpecField returned error %q, want to contain %q", err.Error(), "read ResourceGraphDefinition spec.schema.spec")
	}
}
