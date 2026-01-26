package kro

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResolveInstanceGVK(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		rgdName          string
		schemaAPIVersion string
		schemaKind       string
		want             schema.GroupVersionKind
		wantErr          bool
	}{
		{
			name:             "groupless apiVersion defaults to kro.run",
			rgdName:          "webapp-basic.kro.run",
			schemaAPIVersion: "v1alpha1",
			schemaKind:       "WebApp",
			want:             schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "WebApp"},
		},
		{
			name:             "explicit kro.run group stays kro.run",
			rgdName:          "eks-control-plane.kro.run",
			schemaAPIVersion: "kro.run/v1alpha1",
			schemaKind:       "EKSControlPlane",
			want:             schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "EKSControlPlane"},
		},
		{
			name:             "non-kro group is preserved",
			rgdName:          "custom.example.com",
			schemaAPIVersion: "example.com/v1alpha1",
			schemaKind:       "CustomKind",
			want:             schema.GroupVersionKind{Group: "example.com", Version: "v1alpha1", Kind: "CustomKind"},
		},
		{
			name:             "apiVersion tolerates surrounding whitespace",
			rgdName:          "whitespace.kro.run",
			schemaAPIVersion: "  example.com / v1alpha1  ",
			schemaKind:       "  KindWithSpace  ",
			want:             schema.GroupVersionKind{Group: "example.com", Version: "v1alpha1", Kind: "KindWithSpace"},
		},
		{
			name:       "missing schema apiVersion returns error",
			rgdName:    "missing-apiversion.kro.run",
			schemaKind: "WebApp",
			wantErr:    true,
		},
		{
			name:             "missing schema kind returns error",
			rgdName:          "missing-kind.kro.run",
			schemaAPIVersion: "v1alpha1",
			wantErr:          true,
		},
		{
			name:             "invalid apiVersion returns error",
			rgdName:          "invalid-apiversion.kro.run",
			schemaAPIVersion: "a/b/c",
			schemaKind:       "WebApp",
			wantErr:          true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			scheme := runtime.NewScheme()
			rgdGVK := schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "ResourceGraphDefinition"}
			scheme.AddKnownTypeWithName(rgdGVK, &unstructured.Unstructured{})

			rgd := newRGD(tt.rgdName, tt.schemaAPIVersion, tt.schemaKind)
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rgd).Build()

			got, err := ResolveInstanceGVK(context.Background(), c, tt.rgdName)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveInstanceGVK() error=%v wantErr=%v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			if got != tt.want {
				t.Fatalf("ResolveInstanceGVK() got=%v want=%v", got, tt.want)
			}
		})
	}
}

func TestResolveInstanceGVK_RGDNotFound(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	rgdGVK := schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "ResourceGraphDefinition"}
	scheme.AddKnownTypeWithName(rgdGVK, &unstructured.Unstructured{})
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	_, err := ResolveInstanceGVK(context.Background(), c, "does-not-exist")
	if err == nil {
		t.Fatalf("ResolveInstanceGVK() expected error, got nil")
	}
}

func TestNormalizeSchemaAPIVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		apiVersion string
		wantGroup  string
		wantVer    string
		wantErr    bool
	}{
		{name: "groupless", apiVersion: "v1alpha1", wantGroup: "kro.run", wantVer: "v1alpha1"},
		{name: "with group", apiVersion: "example.com/v1alpha1", wantGroup: "example.com", wantVer: "v1alpha1"},
		{name: "empty group treated as kro.run", apiVersion: "/v1alpha1", wantGroup: "kro.run", wantVer: "v1alpha1"},
		{name: "invalid", apiVersion: "a/b/c", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotGroup, gotVer, err := normalizeSchemaAPIVersion(tt.apiVersion)
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizeSchemaAPIVersion() error=%v wantErr=%v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if gotGroup != tt.wantGroup || gotVer != tt.wantVer {
				t.Fatalf("normalizeSchemaAPIVersion() got=(%q,%q) want=(%q,%q)", gotGroup, gotVer, tt.wantGroup, tt.wantVer)
			}
		})
	}
}

func newRGD(name, schemaAPIVersion, schemaKind string) client.Object {
	obj := map[string]any{
		"apiVersion": "kro.run/v1alpha1",
		"kind":       "ResourceGraphDefinition",
		"metadata": map[string]any{
			"name": name,
		},
		"spec": map[string]any{
			"schema": map[string]any{},
		},
	}
	if schemaAPIVersion != "" {
		obj["spec"].(map[string]any)["schema"].(map[string]any)["apiVersion"] = schemaAPIVersion
	}
	if schemaKind != "" {
		obj["spec"].(map[string]any)["schema"].(map[string]any)["kind"] = schemaKind
	}

	u := &unstructured.Unstructured{Object: obj}
	u.SetGroupVersionKind(schema.GroupVersionKind{Group: "kro.run", Version: "v1alpha1", Kind: "ResourceGraphDefinition"})
	return u
}
