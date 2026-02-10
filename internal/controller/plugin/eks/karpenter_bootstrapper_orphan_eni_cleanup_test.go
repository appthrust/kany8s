package eks

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
)

func TestIsOrphanCNINetworkInterface(t *testing.T) {
	t.Parallel()

	ptr := func(s string) *string { return &s }

	t.Run("true when detached available and tagged", func(t *testing.T) {
		t.Parallel()
		ni := types.NetworkInterface{
			Status: types.NetworkInterfaceStatusAvailable,
			TagSet: []types.Tag{{Key: ptr("eks:eni:owner"), Value: ptr(" amazon-vpc-cni ")}},
		}
		if !isOrphanCNINetworkInterface(ni) {
			t.Fatalf("isOrphanCNINetworkInterface() = false, want true")
		}
	})

	t.Run("false when attachment exists", func(t *testing.T) {
		t.Parallel()
		ni := types.NetworkInterface{
			Attachment: &types.NetworkInterfaceAttachment{AttachmentId: ptr("att-1")},
			Status:     types.NetworkInterfaceStatusAvailable,
			TagSet:     []types.Tag{{Key: ptr("eks:eni:owner"), Value: ptr("amazon-vpc-cni")}},
		}
		if isOrphanCNINetworkInterface(ni) {
			t.Fatalf("isOrphanCNINetworkInterface() = true, want false")
		}
	})

	t.Run("false when status is not available", func(t *testing.T) {
		t.Parallel()
		ni := types.NetworkInterface{
			Status: types.NetworkInterfaceStatusInUse,
			TagSet: []types.Tag{{Key: ptr("eks:eni:owner"), Value: ptr("amazon-vpc-cni")}},
		}
		if isOrphanCNINetworkInterface(ni) {
			t.Fatalf("isOrphanCNINetworkInterface() = true, want false")
		}
	})

	t.Run("false when owner tag is missing", func(t *testing.T) {
		t.Parallel()
		ni := types.NetworkInterface{Status: types.NetworkInterfaceStatusAvailable}
		if isOrphanCNINetworkInterface(ni) {
			t.Fatalf("isOrphanCNINetworkInterface() = true, want false")
		}
	})
}

func TestTagValue(t *testing.T) {
	t.Parallel()

	ptr := func(s string) *string { return &s }
	got := tagValue(
		[]types.Tag{{Key: ptr("a"), Value: ptr(" 1 ")}, {Key: ptr("b"), Value: ptr(" 2 ")}},
		"b",
	)
	if got != "2" {
		t.Fatalf("tagValue() = %q, want %q", got, "2")
	}
}

func TestIgnoreInvalidNetworkInterfaceNotFound(t *testing.T) {
	t.Parallel()

	t.Run("true for InvalidNetworkInterfaceID.NotFound", func(t *testing.T) {
		t.Parallel()
		err := &smithy.GenericAPIError{Code: "InvalidNetworkInterfaceID.NotFound", Message: "not found"}
		if !ignoreInvalidNetworkInterfaceNotFound(err) {
			t.Fatalf("ignoreInvalidNetworkInterfaceNotFound() = false, want true")
		}
	})

	t.Run("false for other API errors", func(t *testing.T) {
		t.Parallel()
		err := &smithy.GenericAPIError{Code: "InvalidNetworkInterfaceID.Malformed", Message: "bad"}
		if ignoreInvalidNetworkInterfaceNotFound(err) {
			t.Fatalf("ignoreInvalidNetworkInterfaceNotFound() = true, want false")
		}
	})

	t.Run("false for non-API errors", func(t *testing.T) {
		t.Parallel()
		if ignoreInvalidNetworkInterfaceNotFound(errors.New("boom")) {
			t.Fatalf("ignoreInvalidNetworkInterfaceNotFound() = true, want false")
		}
	})
}
