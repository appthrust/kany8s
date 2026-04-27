package eks

import "strings"

// safeFargateProfileBaseName returns a Fargate Profile name base that
// satisfies AWS's reserved-prefix rule. AWS EKS reserves the "eks-" prefix
// for FargateProfile and Pod-Execution Role names that AWS auto-creates
// alongside system add-ons; CreateFargateProfile rejects user-supplied names
// that start with that prefix (InvalidParameterException).
//
// Mirrors the same logic in the kany8s-eks-byo ClusterClass RGD
// (rgd-eks-control-plane-byo.yaml: `${schema.metadata.name.startsWith("eks-")
// ? "aioc-" + schema.metadata.name : schema.metadata.name}`). Both writers
// MUST agree on the base name so this controller can find FargateProfile
// objects materialized by the RGD.
//
// See APTH-1576.
func safeFargateProfileBaseName(clusterName string) string {
	if strings.HasPrefix(clusterName, "eks-") {
		return "aioc-" + clusterName
	}
	return clusterName
}
