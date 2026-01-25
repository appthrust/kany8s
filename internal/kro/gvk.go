package kro

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var resourceGraphDefinitionGVK = schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "ResourceGraphDefinition"}

func ResolveInstanceGVK(ctx context.Context, c client.Reader, rgdName string) (schema.GroupVersionKind, error) {
	rgdName = strings.TrimSpace(rgdName)
	if rgdName == "" {
		return schema.GroupVersionKind{}, fmt.Errorf("rgd name is required")
	}

	rgd := &unstructured.Unstructured{}
	rgd.SetGroupVersionKind(resourceGraphDefinitionGVK)
	if err := c.Get(ctx, client.ObjectKey{Name: rgdName}, rgd); err != nil {
		return schema.GroupVersionKind{}, err
	}

	schemaAPIVersion, found, err := unstructured.NestedString(rgd.Object, "spec", "schema", "apiVersion")
	if err != nil {
		return schema.GroupVersionKind{}, fmt.Errorf("read ResourceGraphDefinition spec.schema.apiVersion: %w", err)
	}
	if !found || strings.TrimSpace(schemaAPIVersion) == "" {
		return schema.GroupVersionKind{}, fmt.Errorf("ResourceGraphDefinition %q missing spec.schema.apiVersion", rgdName)
	}

	schemaKind, found, err := unstructured.NestedString(rgd.Object, "spec", "schema", "kind")
	if err != nil {
		return schema.GroupVersionKind{}, fmt.Errorf("read ResourceGraphDefinition spec.schema.kind: %w", err)
	}
	if !found || strings.TrimSpace(schemaKind) == "" {
		return schema.GroupVersionKind{}, fmt.Errorf("ResourceGraphDefinition %q missing spec.schema.kind", rgdName)
	}

	instanceAPIVersion := strings.TrimSpace(schemaAPIVersion)
	instanceKind := strings.TrimSpace(schemaKind)

	var group, version string
	if strings.Contains(instanceAPIVersion, "/") {
		parts := strings.SplitN(instanceAPIVersion, "/", 2)
		group = strings.TrimSpace(parts[0])
		version = strings.TrimSpace(parts[1])
		if group == "" || version == "" {
			return schema.GroupVersionKind{}, fmt.Errorf("invalid ResourceGraphDefinition schema apiVersion %q", instanceAPIVersion)
		}
	} else {
		group = "kro.run"
		version = instanceAPIVersion
	}

	return schema.GroupVersionKind{Group: group, Version: version, Kind: instanceKind}, nil
}
