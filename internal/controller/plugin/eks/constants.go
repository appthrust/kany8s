package eks

import "k8s.io/apimachinery/pkg/runtime/schema"

const (
	reasonACKClusterNotFound = "ACKClusterNotFound"
	reasonACKClusterNotReady = "ACKClusterNotReady"
	reasonRegionNotResolved  = "RegionNotResolved"
	reasonTokenGenerateError = "TokenGenerationFailed"
	reasonSecretOwnership    = "SecretOwnershipConflict"
	reasonSecretTakeover     = "SecretTakeoverApplied"
	reasonPrerequisiteAPI    = "PrerequisiteAPIMissing"
	reasonSecretSynced       = "SecretSynced"

	kany8sControlPlaneKind     = "Kany8sControlPlane"
	kany8sControlPlaneAPIGroup = "controlplane.cluster.x-k8s.io"

	ackClusterNameIndexKey = "eks.kany8s.io/ack-cluster-name"

	karpenterCleanupFinalizer                       = "eks.kany8s.io/karpenter-cleanup"
	karpenterChartVersionAnnotation                 = "eks.kany8s.io/karpenter-chart-version"
	karpenterHelmValuesAnnotation                   = "eks.kany8s.io/karpenter-helm-values-override-json"
	karpenterInterruptionQueueAnnotation            = "eks.kany8s.io/karpenter-interruption-queue"
	karpenterNodePoolTemplateConfigMapAnnotation    = "eks.kany8s.io/karpenter-nodepool-template-configmap"
	karpenterNodePoolTemplateConfigMapKeyAnnotation = "eks.kany8s.io/karpenter-nodepool-template-key"
	oidcThumbprintAutoAnnotation                    = "eks.kany8s.io/oidc-thumbprint-auto"

	topologySubnetIDsVariableName                    = "vpc-subnet-ids"
	topologyControlPlaneSecurityGroupIDsVariableName = "vpc-security-group-ids"
	topologyNodeSecurityGroupIDsVariableName         = "vpc-node-security-group-ids"
	topologyNodeRoleAdditionalPolicyARNsVariableName = "karpenter-node-role-additional-policy-arns"

	defaultKarpenterChartVersion = "1.0.8"
)

var ackClusterGVK = schema.GroupVersionKind{
	Group:   "eks.services.k8s.aws",
	Version: "v1alpha1",
	Kind:    "Cluster",
}
