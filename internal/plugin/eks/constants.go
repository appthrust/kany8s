package eks

import "strings"

const (
	EnableAnnotationKey   = "eks.kany8s.io/kubeconfig-rotator"
	EnableAnnotationValue = "enabled"

	EKSClusterNameAnnotationKey = "eks.kany8s.io/cluster-name"
	ACKClusterNameAnnotationKey = "eks.kany8s.io/ack-cluster-name"
	RegionAnnotationKey         = "eks.kany8s.io/region"

	ManagedByAnnotationKey   = "eks.kany8s.io/managed-by"
	ManagedByAnnotationValue = "eks-kubeconfig-rotator"

	AllowUnmanagedTakeoverAnnotationKey   = "eks.kany8s.io/allow-unmanaged-takeover"
	AllowUnmanagedTakeoverAnnotationValue = "enabled"

	TokenExpirationAnnotationKey = "eks.kany8s.io/token-expiration-rfc3339"
)

const ACKRegionMetadataAnnotationKey = "services.k8s.aws/region"

func IsUnmanagedTakeoverEnabled(annotations map[string]string) bool {
	if len(annotations) == 0 {
		return false
	}
	value := strings.TrimSpace(annotations[AllowUnmanagedTakeoverAnnotationKey])
	return strings.EqualFold(value, AllowUnmanagedTakeoverAnnotationValue) || strings.EqualFold(value, "true")
}
