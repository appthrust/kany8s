package v1alpha1

import (
	"reflect"
	"testing"
)

func TestKany8sControlPlaneSpec_MVPFields(t *testing.T) {
	t.Parallel()

	specType := reflect.TypeFor[Kany8sControlPlaneSpec]()

	if _, ok := specType.FieldByName("Foo"); ok {
		t.Errorf("Kany8sControlPlaneSpec should not include scaffold field Foo")
	}

	assertField(t, specType, "Version", "version", func(f reflect.StructField) error {
		if f.Type.Kind() != reflect.String {
			return newTypeErr(f.Type.String(), "string")
		}
		return nil
	})

	rgdRefField, ok := assertField(t, specType, "ResourceGraphDefinitionRef", "resourceGraphDefinitionRef", nil)
	if ok {
		rgdRefType := rgdRefField.Type
		if rgdRefType.Kind() != reflect.Struct {
			t.Errorf("ResourceGraphDefinitionRef should be a struct, got %s", rgdRefType.String())
		} else {
			assertField(t, rgdRefType, "Name", "name", func(f reflect.StructField) error {
				if f.Type.Kind() != reflect.String {
					return newTypeErr(f.Type.String(), "string")
				}
				return nil
			})
		}
	}

	assertField(t, specType, "KroSpec", "kroSpec,omitempty", func(f reflect.StructField) error {
		if f.Type.Kind() != reflect.Ptr {
			return newTypeErr(f.Type.String(), "*apiextensionsv1.JSON")
		}
		got := f.Type.Elem()
		if got.Name() != "JSON" || got.PkgPath() != "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1" {
			return newTypeErr(f.Type.String(), "*apiextensionsv1.JSON")
		}
		return nil
	})

	assertField(t, specType, "ControlPlaneEndpoint", "controlPlaneEndpoint,omitempty", func(f reflect.StructField) error {
		if f.Type.Kind() != reflect.Struct {
			return newTypeErr(f.Type.String(), "clusterv1.APIEndpoint")
		}
		got := f.Type
		if got.Name() != "APIEndpoint" || got.PkgPath() != "sigs.k8s.io/cluster-api/api/core/v1beta2" {
			return newTypeErr(f.Type.String(), "clusterv1.APIEndpoint")
		}
		return nil
	})
}

type fieldErr struct {
	what string
	got  string
	want string
}

func newTypeErr(got, want string) error {
	return fieldErr{what: "type", got: got, want: want}
}

func (e fieldErr) Error() string {
	return e.what + " mismatch: got " + e.got + ", want " + e.want
}

func assertField(t *testing.T, typ reflect.Type, name, wantJSONTag string, validate func(reflect.StructField) error) (reflect.StructField, bool) {
	t.Helper()

	f, ok := typ.FieldByName(name)
	if !ok {
		t.Errorf("%s missing field %q", typ.Name(), name)
		return reflect.StructField{}, false
	}

	if got := f.Tag.Get("json"); got != wantJSONTag {
		t.Errorf("%s.%s json tag mismatch: got %q, want %q", typ.Name(), name, got, wantJSONTag)
	}

	if validate != nil {
		if err := validate(f); err != nil {
			t.Errorf("%s.%s %s", typ.Name(), name, err)
		}
	}

	return f, true
}
