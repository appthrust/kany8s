package kro

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func SchemaHasSpecField(ctx context.Context, c client.Reader, rgdName string, fieldPath string) (bool, error) {
	rgdName = strings.TrimSpace(rgdName)
	if rgdName == "" {
		return false, fmt.Errorf("rgd name is required")
	}

	fieldPath = strings.TrimSpace(fieldPath)
	if fieldPath == "" {
		return false, fmt.Errorf("field path is required")
	}

	rawParts := strings.Split(fieldPath, ".")
	parts := make([]string, 0, len(rawParts))
	for _, raw := range rawParts {
		p := strings.TrimSpace(raw)
		if p == "" {
			return false, fmt.Errorf("field path %q is invalid", fieldPath)
		}
		parts = append(parts, p)
	}

	rgd := &unstructured.Unstructured{}
	rgd.SetGroupVersionKind(resourceGraphDefinitionGVK)
	if err := c.Get(ctx, client.ObjectKey{Name: rgdName}, rgd); err != nil {
		return false, err
	}

	specSchema, found, err := unstructured.NestedMap(rgd.Object, "spec", "schema", "spec")
	if err != nil {
		return false, fmt.Errorf("read ResourceGraphDefinition spec.schema.spec: %w", err)
	}
	if !found || len(specSchema) == 0 {
		return false, nil
	}

	cur := specSchema
	for i, part := range parts {
		v, ok := cur[part]
		if !ok {
			return false, nil
		}
		if i == len(parts)-1 {
			return true, nil
		}

		next, ok := v.(map[string]any)
		if !ok {
			return false, nil
		}
		cur = next
	}

	return false, nil
}
