package kro

import (
	"errors"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// InstanceStatus is the minimal, provider-agnostic status contract that Kany8s
// consumes from a kro instance.
//
// Each field is optional (nil) if missing, empty, or not a supported type.
type InstanceStatus struct {
	Ready    *bool
	Endpoint *string
	Reason   *string
	Message  *string
}

// ReadInstanceStatus reads the normalized status fields from a kro instance.
//
// It never panics, even if fields are missing or have unexpected types.
// When a field is present but has an unexpected type, the field is ignored and
// the returned error contains details.
func ReadInstanceStatus(u *unstructured.Unstructured) (InstanceStatus, error) {
	var out InstanceStatus
	if u == nil || u.Object == nil {
		return out, nil
	}

	var errs []error

	if v, found, err := unstructured.NestedBool(u.Object, "status", "ready"); err != nil {
		errs = append(errs, fmt.Errorf("read status.ready: %w", err))
	} else if found {
		out.Ready = &v
	}

	if v, ok, err := readStatusString(u, "endpoint"); err != nil {
		errs = append(errs, fmt.Errorf("read status.endpoint: %w", err))
	} else if ok {
		out.Endpoint = &v
	}

	if v, ok, err := readStatusString(u, "reason"); err != nil {
		errs = append(errs, fmt.Errorf("read status.reason: %w", err))
	} else if ok {
		out.Reason = &v
	}

	if v, ok, err := readStatusString(u, "message"); err != nil {
		errs = append(errs, fmt.Errorf("read status.message: %w", err))
	} else if ok {
		out.Message = &v
	}

	if len(errs) > 0 {
		return out, errors.Join(errs...)
	}
	return out, nil
}

func readStatusString(u *unstructured.Unstructured, field string) (v string, ok bool, err error) {
	if u == nil || u.Object == nil {
		return "", false, nil
	}
	got, found, err := unstructured.NestedString(u.Object, "status", field)
	if err != nil {
		return "", false, err
	}
	if !found {
		return "", false, nil
	}
	got = strings.TrimSpace(got)
	if got == "" {
		return "", false, nil
	}
	return got, true, nil
}
