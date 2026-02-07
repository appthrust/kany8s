package eks

import "k8s.io/apimachinery/pkg/runtime/schema"

const (
	reasonACKClusterNotFound = "ACKClusterNotFound"
	reasonACKClusterNotReady = "ACKClusterNotReady"
	reasonRegionNotResolved  = "RegionNotResolved"
	reasonTokenGenerateError = "TokenGenerationFailed"
	reasonSecretOwnership    = "SecretOwnershipConflict"
	reasonSecretSynced       = "SecretSynced"

	kany8sControlPlaneKind     = "Kany8sControlPlane"
	kany8sControlPlaneAPIGroup = "controlplane.cluster.x-k8s.io"
)

var ackClusterGVK = schema.GroupVersionKind{
	Group:   "eks.services.k8s.aws",
	Version: "v1alpha1",
	Kind:    "Cluster",
}
