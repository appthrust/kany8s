package kro

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	rgdGroup   = "kro.run"
	rgdVersion = "v1alpha1"
	rgdKind    = "ResourceGraphDefinition"
)

// ResolveInstanceGVK resolves the generated instance GVK from a kro ResourceGraphDefinition (RGD).
//
// It derives the instance GVK from `.spec.schema.apiVersion` and `.spec.schema.kind`.
// If `.spec.schema.apiVersion` does not include a group (e.g. `v1alpha1`), it is treated as `kro.run/<version>`.
func ResolveInstanceGVK(ctx context.Context, c client.Client, rgdName string) (schema.GroupVersionKind, error) {
	if strings.TrimSpace(rgdName) == "" {
		return schema.GroupVersionKind{}, fmt.Errorf("rgdName is required")
	}

	rgd := &unstructured.Unstructured{}
	rgd.SetGroupVersionKind(schema.GroupVersionKind{Group: rgdGroup, Version: rgdVersion, Kind: rgdKind})
	if err := c.Get(ctx, client.ObjectKey{Name: rgdName}, rgd); err != nil {
		return schema.GroupVersionKind{}, fmt.Errorf("get %s %q: %w", rgdKind, rgdName, err)
	}

	schemaAPIVersion, found, err := unstructured.NestedString(rgd.Object, "spec", "schema", "apiVersion")
	if err != nil {
		return schema.GroupVersionKind{}, fmt.Errorf("read %s %q spec.schema.apiVersion: %w", rgdKind, rgdName, err)
	}
	if !found {
		return schema.GroupVersionKind{}, fmt.Errorf("%s %q missing spec.schema.apiVersion", rgdKind, rgdName)
	}

	schemaKind, found, err := unstructured.NestedString(rgd.Object, "spec", "schema", "kind")
	if err != nil {
		return schema.GroupVersionKind{}, fmt.Errorf("read %s %q spec.schema.kind: %w", rgdKind, rgdName, err)
	}
	if !found {
		return schema.GroupVersionKind{}, fmt.Errorf("%s %q missing spec.schema.kind", rgdKind, rgdName)
	}

	gvk, err := schemaToInstanceGVK(schemaAPIVersion, schemaKind)
	if err != nil {
		return schema.GroupVersionKind{}, fmt.Errorf("resolve instance gvk from %s %q: %w", rgdKind, rgdName, err)
	}
	return gvk, nil
}

func schemaToInstanceGVK(apiVersion, kind string) (schema.GroupVersionKind, error) {
	apiVersion = strings.TrimSpace(apiVersion)
	kind = strings.TrimSpace(kind)

	if apiVersion == "" {
		return schema.GroupVersionKind{}, fmt.Errorf("spec.schema.apiVersion is required")
	}
	if kind == "" {
		return schema.GroupVersionKind{}, fmt.Errorf("spec.schema.kind is required")
	}

	group, version, err := normalizeSchemaAPIVersion(apiVersion)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}

	return schema.GroupVersionKind{Group: group, Version: version, Kind: kind}, nil
}

func normalizeSchemaAPIVersion(apiVersion string) (group, version string, err error) {
	parts := strings.Split(apiVersion, "/")
	switch len(parts) {
	case 1:
		version = strings.TrimSpace(parts[0])
		if version == "" {
			return "", "", fmt.Errorf("invalid apiVersion %q: empty version", apiVersion)
		}
		return rgdGroup, version, nil
	case 2:
		group = strings.TrimSpace(parts[0])
		version = strings.TrimSpace(parts[1])
		if group == "" {
			group = rgdGroup
		}
		if version == "" {
			return "", "", fmt.Errorf("invalid apiVersion %q: empty version", apiVersion)
		}
		return group, version, nil
	default:
		return "", "", fmt.Errorf("invalid apiVersion %q: expected <group>/<version> or <version>", apiVersion)
	}
}
