package v1alpha1

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const (
	metav1ConditionTypeName = "Condition"
	metav1ConditionPkgPath  = "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	rgdRefField, ok := assertField(t, specType, "ResourceGraphDefinitionRef", "resourceGraphDefinitionRef,omitempty", nil)
	if ok {
		rgdRefType := rgdRefField.Type
		if rgdRefType.Kind() != reflect.Ptr {
			t.Errorf("ResourceGraphDefinitionRef should be a pointer, got %s", rgdRefType.String())
		} else {
			elem := rgdRefType.Elem()
			if elem.Kind() != reflect.Struct {
				t.Errorf("ResourceGraphDefinitionRef should point to a struct, got %s", elem.String())
			} else {
				assertField(t, elem, "Name", "name", func(f reflect.StructField) error {
					if f.Type.Kind() != reflect.String {
						return newTypeErr(f.Type.String(), "string")
					}
					return nil
				})
			}
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

func TestKany8sControlPlaneStatus_MVPFields(t *testing.T) {
	t.Parallel()

	statusType := reflect.TypeFor[Kany8sControlPlaneStatus]()

	initField, ok := assertField(t, statusType, "Initialization", "initialization,omitzero", func(f reflect.StructField) error {
		if f.Type.Kind() != reflect.Struct {
			return newTypeErr(f.Type.String(), "Kany8sControlPlaneInitializationStatus")
		}
		return nil
	})
	if ok {
		initType := initField.Type
		if initType.Kind() == reflect.Struct {
			assertField(t, initType, "ControlPlaneInitialized", "controlPlaneInitialized,omitempty", func(f reflect.StructField) error {
				if f.Type.Kind() != reflect.Bool {
					return newTypeErr(f.Type.String(), "bool")
				}
				return nil
			})
		}
	}

	assertField(t, statusType, "Conditions", "conditions,omitempty", func(f reflect.StructField) error {
		if f.Type.Kind() != reflect.Slice {
			return newTypeErr(f.Type.String(), "[]metav1.Condition")
		}
		got := f.Type.Elem()
		if got.Name() != metav1ConditionTypeName || got.PkgPath() != metav1ConditionPkgPath {
			return newTypeErr(f.Type.String(), "[]metav1.Condition")
		}
		return nil
	})

	assertField(t, statusType, "FailureReason", "failureReason,omitempty", func(f reflect.StructField) error {
		if f.Type.Kind() != reflect.Ptr || f.Type.Elem().Kind() != reflect.String {
			return newTypeErr(f.Type.String(), "*string")
		}
		return nil
	})

	assertField(t, statusType, "FailureMessage", "failureMessage,omitempty", func(f reflect.StructField) error {
		if f.Type.Kind() != reflect.Ptr || f.Type.Elem().Kind() != reflect.String {
			return newTypeErr(f.Type.String(), "*string")
		}
		return nil
	})
}

func TestKany8sControlPlane_ConditionsAccessors(t *testing.T) {
	t.Parallel()

	typ := reflect.TypeFor[*Kany8sControlPlane]()

	get, ok := typ.MethodByName("GetConditions")
	if !ok {
		t.Fatalf("Kany8sControlPlane should implement GetConditions")
	}
	if get.Type.NumIn() != 1 || get.Type.NumOut() != 1 {
		t.Fatalf("GetConditions signature mismatch: got %s", get.Type.String())
	}
	getOut := get.Type.Out(0)
	if getOut.Kind() != reflect.Slice {
		t.Fatalf("GetConditions return type mismatch: got %s, want []metav1.Condition", getOut.String())
	}
	getOutElem := getOut.Elem()
	if getOutElem.Name() != metav1ConditionTypeName || getOutElem.PkgPath() != metav1ConditionPkgPath {
		t.Fatalf("GetConditions return type mismatch: got %s, want []metav1.Condition", getOut.String())
	}

	set, ok := typ.MethodByName("SetConditions")
	if !ok {
		t.Fatalf("Kany8sControlPlane should implement SetConditions")
	}
	if set.Type.NumIn() != 2 || set.Type.NumOut() != 0 {
		t.Fatalf("SetConditions signature mismatch: got %s", set.Type.String())
	}
	setIn := set.Type.In(1)
	if setIn.Kind() != reflect.Slice {
		t.Fatalf("SetConditions arg type mismatch: got %s, want []metav1.Condition", setIn.String())
	}
	setInElem := setIn.Elem()
	if setInElem.Name() != metav1ConditionTypeName || setInElem.PkgPath() != metav1ConditionPkgPath {
		t.Fatalf("SetConditions arg type mismatch: got %s, want []metav1.Condition", setIn.String())
	}
}

func TestKany8sControlPlane_PrintColumnsMarkers(t *testing.T) {
	t.Parallel()

	root := findRepoRoot(t)
	path := filepath.Join(root, "api", "v1alpha1", "kany8scontrolplane_types.go")
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	content := string(bytes)

	required := []string{
		"+kubebuilder:printcolumn:name=\"INITIALIZED\"",
		"JSONPath=\".status.initialization.controlPlaneInitialized\"",
		"+kubebuilder:printcolumn:name=\"ENDPOINT\"",
		"JSONPath=\".spec.controlPlaneEndpoint.host\"",
	}
	for _, want := range required {
		if !strings.Contains(content, want) {
			t.Errorf("kany8scontrolplane_types.go missing printcolumn marker %q", want)
		}
	}
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

func findRepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found from %q", dir)
		}
		dir = parent
	}
}
