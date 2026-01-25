package kro

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestReadInstanceStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		obj     map[string]any
		want    InstanceStatus
		wantErr bool
	}{
		{
			name: "missing status returns zero values",
			obj:  map[string]any{},
			want: InstanceStatus{},
		},
		{
			name: "reads status fields",
			obj: map[string]any{
				"status": map[string]any{
					"ready":    true,
					"endpoint": "example.com:6443",
					"reason":   "Ready",
					"message":  "control plane is ready",
				},
			},
			want: InstanceStatus{
				Ready:    true,
				Endpoint: "example.com:6443",
				Reason:   "Ready",
				Message:  "control plane is ready",
			},
		},
		{
			name: "missing optional fields keeps defaults",
			obj: map[string]any{
				"status": map[string]any{
					"ready": true,
				},
			},
			want: InstanceStatus{Ready: true},
		},
		{
			name: "invalid ready type returns error",
			obj: map[string]any{
				"status": map[string]any{
					"ready": "true",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid endpoint type returns error",
			obj: map[string]any{
				"status": map[string]any{
					"endpoint": 123,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			instance := &unstructured.Unstructured{Object: tt.obj}
			got, err := ReadInstanceStatus(instance)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ReadInstanceStatus error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}

			if got != tt.want {
				t.Fatalf("ReadInstanceStatus returned %#v, want %#v", got, tt.want)
			}
		})
	}
}
