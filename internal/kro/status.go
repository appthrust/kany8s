package kro

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type InstanceStatus struct {
	Ready    bool
	Endpoint string
	Reason   string
	Message  string
	Terminal bool
}

func ReadInstanceStatus(instance *unstructured.Unstructured) (InstanceStatus, error) {
	var s InstanceStatus
	if instance == nil {
		return s, fmt.Errorf("instance is nil")
	}

	ready, found, err := unstructured.NestedBool(instance.Object, "status", "ready")
	if err != nil {
		return s, fmt.Errorf("read status.ready: %w", err)
	}
	if found {
		s.Ready = ready
	}

	endpoint, found, err := unstructured.NestedString(instance.Object, "status", "endpoint")
	if err != nil {
		return s, fmt.Errorf("read status.endpoint: %w", err)
	}
	if found {
		s.Endpoint = endpoint
	}

	reason, found, err := unstructured.NestedString(instance.Object, "status", "reason")
	if err != nil {
		return s, fmt.Errorf("read status.reason: %w", err)
	}
	if found {
		s.Reason = reason
	}

	message, found, err := unstructured.NestedString(instance.Object, "status", "message")
	if err != nil {
		return s, fmt.Errorf("read status.message: %w", err)
	}
	if found {
		s.Message = message
	}

	terminal, found, err := unstructured.NestedBool(instance.Object, "status", "terminal")
	if err != nil {
		return s, fmt.Errorf("read status.terminal: %w", err)
	}
	if found {
		s.Terminal = terminal
	}

	return s, nil
}
