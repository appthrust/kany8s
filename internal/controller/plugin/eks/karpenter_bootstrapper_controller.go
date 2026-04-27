package eks

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/reoring/kany8s/internal/kubeconfig"

	coreeks "github.com/reoring/kany8s/internal/plugin/eks"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const mutationErrorObjectKey = "eks.kany8s.io/internal-mutation-error"

const (
	karpenterEnableLabelKey   = "eks.kany8s.io/karpenter"
	karpenterEnableLabelValue = "enabled"

	karpenterManagedByValue = "eks-karpenter-bootstrapper"

	capiClusterNameLabelKey = "cluster.x-k8s.io/cluster-name"
)

var (
	ackAccessEntryGVK        = schema.GroupVersionKind{Group: "eks.services.k8s.aws", Version: "v1alpha1", Kind: "AccessEntry"}
	ackFargateProfileGVK     = schema.GroupVersionKind{Group: "eks.services.k8s.aws", Version: "v1alpha1", Kind: "FargateProfile"}
	ackIAMPolicyGVK          = schema.GroupVersionKind{Group: "iam.services.k8s.aws", Version: "v1alpha1", Kind: "Policy"}
	ackIAMRoleGVK            = schema.GroupVersionKind{Group: "iam.services.k8s.aws", Version: "v1alpha1", Kind: "Role"}
	ackIAMInstanceProfileGVK = schema.GroupVersionKind{Group: "iam.services.k8s.aws", Version: "v1alpha1", Kind: "InstanceProfile"}
	ackOIDCProviderGVK       = schema.GroupVersionKind{Group: "iam.services.k8s.aws", Version: "v1alpha1", Kind: "OpenIDConnectProvider"}
	ackSecurityGroupGVK      = schema.GroupVersionKind{Group: "ec2.services.k8s.aws", Version: "v1alpha1", Kind: "SecurityGroup"}

	fluxOCIRepositoryGVK = schema.GroupVersionKind{Group: "source.toolkit.fluxcd.io", Version: "v1beta2", Kind: "OCIRepository"}
	fluxHelmReleaseGVK   = schema.GroupVersionKind{Group: "helm.toolkit.fluxcd.io", Version: "v2", Kind: "HelmRelease"}

	clusterResourceSetGVK = schema.GroupVersionKind{Group: "addons.cluster.x-k8s.io", Version: "v1beta2", Kind: "ClusterResourceSet"}

	errNodePoolTemplateInvalid  = errors.New("nodepool template invalid")
	errOIDCThumbprintUnverified = errors.New("oidc thumbprint chain unverified")
)

const (
	reasonKarpenterDisabled         = "KarpenterDisabled"
	reasonTopologyMissing           = "TopologyMissing"
	reasonTopologyVariableMissing   = "TopologyVariableMissing"
	reasonTopologyVariablePatched   = "TopologyVariablePatched"
	reasonKarpenterValuesInvalid    = "KarpenterValuesInvalid"
	reasonNodePoolTemplateInvalid   = "NodePoolTemplateInvalid"
	reasonACKClusterNotFoundKarp    = "ACKClusterNotFound"
	reasonACKClusterNotReadyKarp    = "ACKClusterNotReady"
	reasonFluxNotInstalled          = "FluxNotInstalled"
	reasonFluxSuspended             = "FluxSuspendedForDeletion"
	reasonResourceOwnership         = "ResourceOwnershipConflict"
	reasonBootstrapperReconciled    = "BootstrapperReconciled"
	reasonWorkloadRolloutRestarted  = "WorkloadRolloutRestarted"
	reasonAWSPrerequisitesNotReady  = "AWSPrerequisitesNotReady"
	reasonNodeSecurityGroupNotReady = "NodeSecurityGroupNotReady"
	reasonOrphanENICleanup          = "OrphanENICleanup"
	reasonResourceTakeover          = "ResourceTakeoverApplied"
	reasonOIDCThumbprintSkipped     = "OIDCThumbprintSkipped"
)

type EKSKarpenterBootstrapperReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	Recorder       recordEventEmitter
	TokenGenerator coreeks.TokenGenerator
	Now            func() time.Time
	RESTMapper     meta.RESTMapper

	FailureBackoff     time.Duration
	SteadyStateRequeue time.Duration
	KarpenterChartTag  string
	CleanupFinalizer   string

	ValidateSubnets        func(ctx context.Context, region string, subnetIDs []string) (fargateSubnetValidationResult, error)
	ResolveOIDCThumbprints func(ctx context.Context, issuerURL string) ([]string, error)
}

// nolint:gocyclo
func (r *EKSKarpenterBootstrapperReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, retErr error) {
	defer func() {
		if retErr != nil {
			recordReconcileError(metricControllerBootstrapper)
		}
	}()

	cluster := &clusterv1.Cluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	capiClusterName, eksClusterName, ackClusterName := resolveClusterNames(cluster)
	log := logf.FromContext(ctx).WithValues(
		"cluster", req.String(),
		"eksClusterName", eksClusterName,
		"ackClusterName", ackClusterName,
	)
	ctx = logf.IntoContext(ctx, log)

	if cluster.DeletionTimestamp != nil {
		if !controllerutil.ContainsFinalizer(cluster, r.cleanupFinalizer()) {
			return ctrl.Result{}, nil
		}
		result, done := r.reconcileDeletingCluster(ctx, cluster, capiClusterName, eksClusterName, ackClusterName)
		if !done {
			return result, nil
		}
		if err := r.removeCleanupFinalizer(ctx, cluster); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if !isKarpenterEnabled(cluster) {
		return ctrl.Result{}, nil
	}
	if err := r.ensureCleanupFinalizer(ctx, cluster); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.ensureClusterNameLabel(ctx, cluster); err != nil {
		return ctrl.Result{}, err
	}

	if !cluster.Spec.Topology.IsDefined() {
		msg := fmt.Sprintf(
			"Cluster.spec.topology is required; cause: BYO bootstrap needs topology variable %q (required); %q and %q are optional (bootstrapper creates/patches security groups when they are empty). action: set Cluster.spec.topology.variables before enabling karpenter bootstrap",
			topologyNodeSubnetIDsVariableName,
			topologyNodeSecurityGroupIDsVariableName,
			topologyControlPlaneSecurityGroupIDsVariableName,
		)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonTopologyMissing, msg)
		return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
	}

	nodeSubnetIDs, ok, err := readTopologyStringSlice(cluster, topologyNodeSubnetIDsVariableName)
	if err != nil {
		return ctrl.Result{}, err
	}
	nodeSubnetIDs = normalizeDistinctStrings(nodeSubnetIDs)
	if !ok || len(nodeSubnetIDs) == 0 {
		msg := fmt.Sprintf(
			"missing/invalid topology variable %q; cause: karpenter Fargate + NodePool require at least one private subnet ID. action: set Cluster.spec.topology.variables[%q] to private subnet IDs (NAT egress or VPC endpoints recommended for image pulls; >=2 across >=2 AZs recommended for HA)",
			topologyNodeSubnetIDsVariableName,
			topologyNodeSubnetIDsVariableName,
		)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonTopologyVariableMissing, msg)
		return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
	}

	controlPlaneSecurityGroupIDs, ok, err := readTopologyStringSlice(cluster, topologyControlPlaneSecurityGroupIDsVariableName)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !ok {
		controlPlaneSecurityGroupIDs = nil
	}
	controlPlaneSecurityGroupIDs = normalizeDistinctStrings(controlPlaneSecurityGroupIDs)

	nodeSecurityGroupIDs := controlPlaneSecurityGroupIDs
	if ids, found, err := readTopologyStringSlice(cluster, topologyNodeSecurityGroupIDsVariableName); err != nil {
		return ctrl.Result{}, err
	} else if found {
		nodeSecurityGroupIDs = normalizeDistinctStrings(ids)
	}

	requiredAPIs := []schema.GroupVersionKind{
		ackClusterGVK,
		ackOIDCProviderGVK,
		ackIAMPolicyGVK,
		ackIAMRoleGVK,
		ackIAMInstanceProfileGVK,
		ackAccessEntryGVK,
		ackFargateProfileGVK,
	}
	if len(nodeSecurityGroupIDs) == 0 {
		requiredAPIs = append(requiredAPIs, ackSecurityGroupGVK)
	}
	if missing := r.missingAPIs(requiredAPIs...); len(missing) > 0 {
		msg := fmt.Sprintf(
			"missing prerequisite APIs %s; cause: ACK resources cannot be reconciled. action: install ACK EKS/IAM CRDs (and ACK EC2 SecurityGroup CRD when %q is empty)",
			joinGVKs(missing),
			topologyNodeSecurityGroupIDsVariableName,
		)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonPrerequisiteAPI, msg)
		return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
	}

	fluxAPIsAvailable := r.areAPIsAvailable(fluxOCIRepositoryGVK, fluxHelmReleaseGVK)
	crsAPIAvailable := r.isAPIAvailable(clusterResourceSetGVK)

	ackCluster, err := r.getACKCluster(ctx, cluster.Namespace, ackClusterName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("waiting for ACK EKS Cluster %s/%s", cluster.Namespace, ackClusterName)
			r.emitEvent(cluster, corev1.EventTypeNormal, reasonACKClusterNotFoundKarp, msg)
			return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
		}
		return ctrl.Result{}, err
	}

	endpoint, _ := readNestedString(ackCluster.Object, "status", "endpoint")
	issuerURL, _ := readNestedString(ackCluster.Object, "status", "identity", "oidc", "issuer")
	accountID, _ := readNestedString(ackCluster.Object, "status", "ackResourceMetadata", "ownerAccountID")
	clusterStatus, _ := readNestedString(ackCluster.Object, "status", "status")

	if endpoint == "" || issuerURL == "" || accountID == "" {
		msg := fmt.Sprintf("waiting for ACK EKS status fields endpoint/identity.oidc.issuer/ownerAccountID on %s/%s", cluster.Namespace, ackClusterName)
		r.emitEvent(cluster, corev1.EventTypeNormal, reasonACKClusterNotReadyKarp, msg)
		return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
	}
	if !strings.EqualFold(clusterStatus, "ACTIVE") {
		msg := fmt.Sprintf("waiting for ACK EKS Cluster status.status to become ACTIVE (current=%q) on %s/%s", clusterStatus, cluster.Namespace, ackClusterName)
		r.emitEvent(cluster, corev1.EventTypeNormal, reasonACKClusterNotReadyKarp, msg)
		return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
	}

	authMode, found := readNestedString(ackCluster.Object, "spec", "accessConfig", "authenticationMode")
	if !found {
		msg := fmt.Sprintf(
			"waiting for ACK EKS Cluster spec.accessConfig.authenticationMode on %s/%s; cause: AccessEntry-based node join requires %q. action: set /spec/accessConfig/authenticationMode=%q",
			cluster.Namespace,
			ackClusterName,
			"API_AND_CONFIG_MAP",
			"API_AND_CONFIG_MAP",
		)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonAWSPrerequisitesNotReady, msg)
		return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
	}
	if authMode != "API_AND_CONFIG_MAP" {
		msg := fmt.Sprintf(
			"EKS accessConfig.authenticationMode mismatch on %s/%s; cause: AccessEntry-based node join requires %q (got %q). action: update ACK EKS Cluster /spec/accessConfig/authenticationMode=%q",
			cluster.Namespace,
			ackClusterName,
			"API_AND_CONFIG_MAP",
			authMode,
			"API_AND_CONFIG_MAP",
		)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonAWSPrerequisitesNotReady, msg)
		return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
	}

	region := resolveRegion(cluster, ackCluster)
	if region == "" {
		msg := fmt.Sprintf(
			"failed to resolve region (checked %q annotation, ACK status.ackResourceMetadata.region, ACK metadata.annotations[%q])",
			coreeks.RegionAnnotationKey,
			coreeks.ACKRegionMetadataAnnotationKey,
		)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonRegionNotResolved, msg)
		return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
	}
	if _, err := r.ensureClusterAnnotation(ctx, cluster, coreeks.RegionAnnotationKey, region); err != nil {
		return ctrl.Result{}, err
	}
	if _, err := r.ensureClusterAnnotation(ctx, cluster, coreeks.EKSClusterNameAnnotationKey, eksClusterName); err != nil {
		return ctrl.Result{}, err
	}
	log = log.WithValues("region", region)
	ctx = logf.IntoContext(ctx, log)

	subnetValidation, err := r.validateFargateSubnets(ctx, region, nodeSubnetIDs)
	if err != nil {
		msg := fmt.Sprintf(
			"invalid topology variable %q: %v; cause: EKS FargateProfile requires private subnets in one VPC. action: pass private subnet IDs in Cluster.spec.topology.variables[%q]; ensure NAT egress or VPC endpoints (ecr.api,ecr.dkr,s3,sts,logs) for image pulls",
			topologyNodeSubnetIDsVariableName,
			err,
			topologyNodeSubnetIDsVariableName,
		)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonAWSPrerequisitesNotReady, msg)
		return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
	}
	if len(subnetValidation.SubnetsWithoutNATDefaultRoute) > 0 {
		msg := fmt.Sprintf(
			"subnets %v have no default route via NAT gateway/instance; cause: image pulls on Fargate/Karpenter may fail without egress. action: add NAT route or VPC endpoints (ecr.api,ecr.dkr,s3,sts,logs)",
			subnetValidation.SubnetsWithoutNATDefaultRoute,
		)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonAWSPrerequisitesNotReady, msg)
	}
	if subnetValidation.DistinctAZCount < 2 {
		msg := fmt.Sprintf(
			"node subnets span only %d AZ; cause: karpenter Fargate + NodePool become single-AZ and lose HA. action: add private+NAT subnets in additional AZs to Cluster.spec.topology.variables[%q]",
			subnetValidation.DistinctAZCount,
			topologyNodeSubnetIDsVariableName,
		)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonAWSPrerequisitesNotReady, msg)
	}

	karpenterChartTag := r.resolveKarpenterChartTag(cluster)

	// Default reconcile cadence, overridden when prerequisites are not ready.
	requeueAfter := r.steadyStateRequeue()
	if !fluxAPIsAvailable {
		msg := "Flux APIs (source.toolkit.fluxcd.io/v1beta2 OCIRepository, helm.toolkit.fluxcd.io/v2 HelmRelease) are not available; cause: required CRD/controller is missing. action: install Flux source-controller + helm-controller"
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonFluxNotInstalled, msg)
		requeueAfter = r.failureBackoff()
	}
	if !crsAPIAvailable {
		msg := "ClusterResourceSet API (addons.cluster.x-k8s.io/v1beta2) is not available; cause: default NodePool/EC2NodeClass distribution cannot be reconciled. action: install CAPI addons components with ClusterResourceSet CRD"
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonPrerequisiteAPI, msg)
		requeueAfter = r.failureBackoff()
	}

	// Ensure we have at least one security group for Karpenter nodes.
	// If the topology variable vpc-node-security-group-ids is empty, we create a default node SecurityGroup via ACK,
	// then inject the created security group ID back into Cluster.spec.topology.variables.
	if len(nodeSecurityGroupIDs) == 0 {
		id, managed, err := r.ensureNodeSecurityGroup(ctx, cluster, region, eksClusterName, nodeSubnetIDs)
		if err != nil {
			msg := fmt.Sprintf("failed to reconcile node SecurityGroup: %v", err)
			r.emitEvent(cluster, corev1.EventTypeWarning, reasonAWSPrerequisitesNotReady, msg)
			return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
		}
		if !managed {
			msg := fmt.Sprintf("node SecurityGroup exists and is not managed by %s", karpenterManagedByValue)
			r.emitEvent(cluster, corev1.EventTypeWarning, reasonResourceOwnership, msg)
			recordOwnershipConflict(metricControllerBootstrapper, "SecurityGroup")
			return ctrl.Result{RequeueAfter: r.steadyStateRequeue()}, nil
		}
		if id == "" {
			msg := "waiting for node SecurityGroup to be created (status.id is empty)"
			r.emitEvent(cluster, corev1.EventTypeNormal, reasonNodeSecurityGroupNotReady, msg)
			requeueAfter = r.failureBackoff()
		} else {
			patchedNode, err := r.ensureTopologyStringSliceVariable(ctx, cluster, topologyNodeSecurityGroupIDsVariableName, []string{id})
			if err != nil {
				return ctrl.Result{}, err
			}
			patchedControlPlane := false
			if len(controlPlaneSecurityGroupIDs) == 0 {
				patchedControlPlane, err = r.ensureTopologyStringSliceVariable(ctx, cluster, topologyControlPlaneSecurityGroupIDsVariableName, []string{id})
				if err != nil {
					return ctrl.Result{}, err
				}
			}
			if patchedNode {
				msg := fmt.Sprintf("patched Cluster.spec.topology.variables[%q] with %q", topologyNodeSecurityGroupIDsVariableName, id)
				r.emitEvent(cluster, corev1.EventTypeNormal, reasonTopologyVariablePatched, msg)
			}
			if patchedControlPlane {
				msg := fmt.Sprintf("patched Cluster.spec.topology.variables[%q] with %q", topologyControlPlaneSecurityGroupIDsVariableName, id)
				r.emitEvent(cluster, corev1.EventTypeNormal, reasonTopologyVariablePatched, msg)
			}
			if patchedNode || patchedControlPlane {
				// Let CAPI Topology / ControlPlane reconcile propagate injected values.
				return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
			}
			nodeSecurityGroupIDs = []string{id}
		}
	}

	issuerHostPath, err := normalizeIssuerHostPath(issuerURL)
	if err != nil {
		return ctrl.Result{}, err
	}
	oidcProviderARN := fmt.Sprintf("arn:aws:iam::%s:oidc-provider/%s", accountID, issuerHostPath)
	thumbprints, err := r.resolveOIDCThumbprints(ctx, cluster, issuerURL)
	if err != nil {
		if errors.Is(err, errOIDCThumbprintUnverified) {
			msg := fmt.Sprintf("skipped OIDC thumbprint auto-setting: %v; continuing without spec.thumbprints. action: ensure issuer certificate chain is verifiable or disable %q annotation", err, oidcThumbprintAutoAnnotation)
			r.emitEvent(cluster, corev1.EventTypeWarning, reasonOIDCThumbprintSkipped, msg)
			thumbprints = nil
		} else {
			msg := fmt.Sprintf("failed to resolve OIDC thumbprint: %v; action: fix issuer reachability or disable %q annotation", err, oidcThumbprintAutoAnnotation)
			r.emitEvent(cluster, corev1.EventTypeWarning, reasonAWSPrerequisitesNotReady, msg)
			return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
		}
	}

	// 1) Ensure OIDC provider for IRSA.
	oidcName := fmt.Sprintf("%s-oidc-provider", capiClusterName)
	if ok, err := r.ensureOIDCProvider(ctx, cluster, oidcName, region, issuerURL, thumbprints); err != nil {
		return ctrl.Result{}, err
	} else if !ok {
		msg := fmt.Sprintf("OIDC provider %s/%s exists and is not managed by %s", cluster.Namespace, oidcName, karpenterManagedByValue)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonResourceOwnership, msg)
		recordOwnershipConflict(metricControllerBootstrapper, "OpenIDConnectProvider")
		return ctrl.Result{RequeueAfter: r.steadyStateRequeue()}, nil
	}

	// 2) Ensure IAM resources.
	controllerPolicyName := fmt.Sprintf("%s-karpenter-controller-policy", capiClusterName)
	controllerRoleName := fmt.Sprintf("%s-karpenter-controller", capiClusterName)
	controllerRoleAWSName := shortenAWSName(fmt.Sprintf("%s-karpenter-controller", eksClusterName), 64)
	nodeRoleName := fmt.Sprintf("%s-karpenter-node", capiClusterName)
	nodeRoleAWSName := shortenAWSName(fmt.Sprintf("%s-node", eksClusterName), 64)
	nodeInstanceProfileName := fmt.Sprintf("%s-karpenter-node-instance-profile", capiClusterName)
	nodeInstanceProfileAWSName := shortenAWSName(fmt.Sprintf("%s-node", eksClusterName), 128)
	// Fargate pod-execution IAM Role + FargateProfile resources are owned by
	// the kany8s-eks-byo ClusterClass ResourceGraphDefinition now (= APTH-1568
	// Path α). The plugin no longer materializes them; see monitor-only block
	// further below for status polling.

	controllerRoleARN := fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, controllerRoleAWSName)
	nodeRoleARN := fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, nodeRoleAWSName)

	policyDoc := buildKarpenterControllerPolicyDocument(region, accountID, eksClusterName, nodeRoleARN)
	if ok, err := r.ensureIAMPolicy(ctx, cluster, controllerPolicyName, region, policyDoc, shortenAWSName(fmt.Sprintf("%s-karpenter-controller", eksClusterName), 128)); err != nil {
		return ctrl.Result{}, err
	} else if !ok {
		msg := fmt.Sprintf("IAM Policy %s/%s exists and is not managed by %s", cluster.Namespace, controllerPolicyName, karpenterManagedByValue)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonResourceOwnership, msg)
		recordOwnershipConflict(metricControllerBootstrapper, "Policy")
		return ctrl.Result{RequeueAfter: r.steadyStateRequeue()}, nil
	}

	assumePolicy := buildKarpenterControllerAssumeRolePolicyDocument(oidcProviderARN, issuerHostPath)
	if ok, err := r.ensureIAMRoleForIRSA(ctx, cluster, controllerRoleName, region, controllerRoleAWSName, assumePolicy, []string{controllerPolicyName}); err != nil {
		return ctrl.Result{}, err
	} else if !ok {
		msg := fmt.Sprintf("IAM Role %s/%s exists and is not managed by %s", cluster.Namespace, controllerRoleName, karpenterManagedByValue)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonResourceOwnership, msg)
		recordOwnershipConflict(metricControllerBootstrapper, "Role")
		return ctrl.Result{RequeueAfter: r.steadyStateRequeue()}, nil
	}

	nodeManagedPolicies := []string{
		"arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy",
		"arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy",
		"arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly",
	}
	if additionalPolicyARNs, ok, err := readTopologyStringSlice(cluster, topologyNodeRoleAdditionalPolicyARNsVariableName); err != nil {
		return ctrl.Result{}, err
	} else if ok {
		nodeManagedPolicies = append(nodeManagedPolicies, additionalPolicyARNs...)
	}
	nodeManagedPolicies = normalizeDistinctStrings(nodeManagedPolicies)
	if ok, err := r.ensureIAMRoleForEC2(ctx, cluster, nodeRoleName, region, nodeRoleAWSName, nodeManagedPolicies); err != nil {
		return ctrl.Result{}, err
	} else if !ok {
		msg := fmt.Sprintf("IAM Role %s/%s exists and is not managed by %s", cluster.Namespace, nodeRoleName, karpenterManagedByValue)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonResourceOwnership, msg)
		recordOwnershipConflict(metricControllerBootstrapper, "Role")
		return ctrl.Result{RequeueAfter: r.steadyStateRequeue()}, nil
	}
	if ok, err := r.ensureIAMInstanceProfile(ctx, cluster, nodeInstanceProfileName, region, nodeInstanceProfileAWSName, nodeRoleName); err != nil {
		return ctrl.Result{}, err
	} else if !ok {
		msg := fmt.Sprintf("IAM InstanceProfile %s/%s exists and is not managed by %s", cluster.Namespace, nodeInstanceProfileName, karpenterManagedByValue)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonResourceOwnership, msg)
		recordOwnershipConflict(metricControllerBootstrapper, "InstanceProfile")
		return ctrl.Result{RequeueAfter: r.steadyStateRequeue()}, nil
	}

	// (Fargate pod-execution IAM Role create moved to ClusterClass RGD; see comment
	// near IAM section above and APTH-1568.)

	// 3) Ensure EKS AccessEntry (node join without aws-auth).
	accessEntryName := fmt.Sprintf("%s-karpenter-node", capiClusterName)
	if ok, err := r.ensureAccessEntry(ctx, cluster, accessEntryName, region, ackClusterName, nodeRoleARN); err != nil {
		return ctrl.Result{}, err
	} else if !ok {
		msg := fmt.Sprintf("AccessEntry %s/%s exists and is not managed by %s", cluster.Namespace, accessEntryName, karpenterManagedByValue)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonResourceOwnership, msg)
		recordOwnershipConflict(metricControllerBootstrapper, "AccessEntry")
		return ctrl.Result{RequeueAfter: r.steadyStateRequeue()}, nil
	}

	// 4) Wait for Fargate profiles created declaratively by the kany8s-eks-byo
	//    ClusterClass ResourceGraphDefinition. The RGD owns FargateProfile and
	//    Fargate Pod Execution IAM Role lifecycle now; the plugin only monitors
	//    status (= isACKFargateProfileActive) and triggers workload restarts
	//    after ACTIVE. See APTH-1568 and the design doc at
	//    knowledge/eks-root-fix-design.md (= reoring/demo0521 PR #28) § 2.1.
	// FargateProfile object names are produced by the kany8s-eks-byo
	// ClusterClass RGD using safeFargateProfileBaseName(...) to dodge AWS's
	// reserved "eks-" prefix on FargateProfile / Pod-Execution Role names.
	// This reader MUST mirror that sanitizer so the lookup keys match the
	// objects materialized by the RGD. See APTH-1576.
	fargateBase := safeFargateProfileBaseName(capiClusterName)
	karpenterFargateObjName := fmt.Sprintf("%s-karpenter", fargateBase)
	corednsFargateObjName := fmt.Sprintf("%s-coredns", fargateBase)

	karpenterFargateActive, err := r.isACKFargateProfileActive(ctx, cluster.Namespace, karpenterFargateObjName)
	if err != nil {
		return ctrl.Result{}, err
	}
	corednsFargateActive, err := r.isACKFargateProfileActive(ctx, cluster.Namespace, corednsFargateObjName)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !karpenterFargateActive || !corednsFargateActive {
		// Surface a Warning event so operators can see why the bootstrap is
		// blocked. The owner-conflict signal that previously fired here was
		// retired together with the in-plugin create logic; without this
		// emit the only signal of "Fargate not ready" was status fields on
		// the plugin CR, invisible to anyone watching CAPI Cluster events.
		msg := fmt.Sprintf(
			"waiting for ClusterClass RGD to provision FargateProfile %q/%q on %s/%s; cause: kany8s-eks-byo ClusterClass owns FargateProfile lifecycle. action: check ResourceGraphDefinition + ACK eks-controller logs if this persists",
			karpenterFargateObjName,
			corednsFargateObjName,
			cluster.Namespace,
			cluster.Name,
		)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonAWSPrerequisitesNotReady, msg)
		requeueAfter = r.failureBackoff()
	}

	helmValues, err := r.resolveKarpenterHelmValues(cluster, eksClusterName, endpoint, controllerRoleARN)
	if err != nil {
		msg := fmt.Sprintf(
			"invalid %q annotation: %v; cause: override must be a JSON object. action: set valid JSON or remove the annotation",
			karpenterHelmValuesAnnotation,
			err,
		)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonKarpenterValuesInvalid, msg)
		return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
	}

	// 5) Flux: install Karpenter chart to workload cluster via remote kubeconfig.
	fluxInstalled := false
	if fluxAPIsAvailable {
		fluxInstalled, err = r.ensureFluxKarpenter(ctx, cluster, capiClusterName, karpenterChartTag, helmValues)
		if err != nil {
			if isNoMatchError(err) {
				msg := "Flux (source-controller/helm-controller) is not installed in the management cluster; cause: Karpenter HelmRelease cannot be reconciled. action: install Flux source-controller + helm-controller or remove the karpenter label"
				r.emitEvent(cluster, corev1.EventTypeWarning, reasonFluxNotInstalled, msg)
				// Keep reconciling AWS resources even without Flux.
				fluxInstalled = false
				requeueAfter = r.failureBackoff()
			} else {
				return ctrl.Result{}, err
			}
		}
	}

	// 6) ClusterResourceSet: apply default NodePool/EC2NodeClass to workload cluster.
	if len(nodeSecurityGroupIDs) > 0 {
		if err := r.ensureDefaultNodePoolResources(ctx, cluster, capiClusterName, eksClusterName, nodeInstanceProfileAWSName, nodeSubnetIDs, nodeSecurityGroupIDs); err != nil {
			if errors.Is(err, errNodePoolTemplateInvalid) {
				msg := fmt.Sprintf(
					"invalid NodePool template: %v; action: fix ConfigMap referenced by %q/%q or remove the annotation to use defaults",
					err,
					karpenterNodePoolTemplateConfigMapAnnotation,
					karpenterNodePoolTemplateConfigMapKeyAnnotation,
				)
				r.emitEvent(cluster, corev1.EventTypeWarning, reasonNodePoolTemplateInvalid, msg)
				return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
			}
			return ctrl.Result{}, err
		}
	}

	// 7) Workload: restart Pods that were created before FargateProfile became ACTIVE.
	if needs, err := r.ensureWorkloadRolloutRestarts(ctx, cluster, capiClusterName, corednsFargateActive, karpenterFargateActive, fluxInstalled); err != nil {
		log.Error(err, "failed to rollout-restart workload deployments", "phase", "workload-restart")
		requeueAfter = r.failureBackoff()
	} else if needs {
		requeueAfter = r.failureBackoff()
	}

	log.V(1).Info(
		"reconciled EKS Karpenter bootstrap",
		"phase", "steady-state",
		"endpoint", endpoint,
		"karpenterChartTag", karpenterChartTag,
		"fluxInstalled", fluxInstalled,
		"karpenterFargateActive", karpenterFargateActive,
		"corednsFargateActive", corednsFargateActive,
	)
	if requeueAfter == r.steadyStateRequeue() {
		// Event noise is undesirable; emit only when we're in steady state.
		r.emitEvent(cluster, corev1.EventTypeNormal, reasonBootstrapperReconciled, "karpenter bootstrap resources reconciled")
	}
	recordSuccessfulSync(metricControllerBootstrapper, r.now())
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func (r *EKSKarpenterBootstrapperReconciler) reconcileDeletingCluster(
	ctx context.Context,
	cluster *clusterv1.Cluster,
	capiClusterName, eksClusterName, ackClusterName string,
) (ctrl.Result, bool) {
	if r == nil || cluster == nil {
		return ctrl.Result{}, true
	}

	log := logf.FromContext(ctx).WithValues("phase", "delete")
	ctx = logf.IntoContext(ctx, log)

	// Suspend Flux resources first so Helm does not race and recreate Karpenter while deleting.
	if suspended, err := r.suspendFluxKarpenterOnDelete(ctx, cluster, capiClusterName); err != nil {
		log.Error(err, "failed to suspend Flux resources")
		return ctrl.Result{RequeueAfter: r.failureBackoff()}, false
	} else if suspended {
		r.emitEvent(cluster, corev1.EventTypeNormal, reasonFluxSuspended, "suspended Flux OCIRepository/HelmRelease for cluster deletion")
	}

	// Resolve region from the Cluster annotation first. ACK Cluster may already be deleting/not-found.
	region := resolveRegion(cluster, nil)
	if strings.TrimSpace(region) == "" {
		if secretRegion, secretEKSName, ok, err := r.readWorkloadKubeconfigMetadata(ctx, cluster.Namespace, capiClusterName); err != nil {
			log.Error(err, "failed to read workload kubeconfig secret metadata")
		} else if ok {
			if strings.TrimSpace(region) == "" {
				region = secretRegion
			}
			if strings.TrimSpace(eksClusterName) == "" {
				eksClusterName = secretEKSName
			}
		}
	}
	if strings.TrimSpace(region) == "" && strings.TrimSpace(ackClusterName) != "" {
		ack, err := r.getACKCluster(ctx, cluster.Namespace, ackClusterName)
		if err == nil {
			region = resolveRegion(cluster, ack)
		}
	}

	// Stop provisioning first to avoid replacement nodes while instances are terminating.
	if err := r.stopWorkloadKarpenterProvisioning(ctx, cluster, capiClusterName, region, eksClusterName, ackClusterName); err != nil {
		log.Error(err, "failed to stop workload Karpenter provisioning")
	}

	if strings.TrimSpace(region) == "" {
		log.Info("skip Karpenter EC2 node cleanup (region not resolved)")
		return ctrl.Result{}, true
	}
	if strings.TrimSpace(eksClusterName) == "" {
		log.Info("skip Karpenter EC2 node cleanup (EKS cluster name not resolved)")
		return ctrl.Result{}, true
	}

	log = log.WithValues("region", region)
	ctx = logf.IntoContext(ctx, log)

	// Terminate instances, with a few retries to reduce the chance of replacement nodes.
	var (
		lastDone        bool
		lastToTerminate int
		lastShutting    int
	)
	for attempt := range 3 {
		done, toTerminate, shuttingDown, err := cleanupKarpenterEC2Instances(ctx, region, eksClusterName)
		if err != nil {
			log.Error(err, "failed to cleanup Karpenter EC2 instances")
			return ctrl.Result{RequeueAfter: r.failureBackoff()}, false
		}
		lastDone, lastToTerminate, lastShutting = done, toTerminate, shuttingDown
		if done || toTerminate == 0 {
			break
		}
		if attempt < 2 {
			time.Sleep(5 * time.Second)
		}
	}
	if !lastDone {
		log.V(1).Info(
			"Karpenter EC2 instances termination in progress",
			"phase", "delete-ec2",
			"toTerminate", lastToTerminate,
			"shuttingDown", lastShutting,
		)
		return ctrl.Result{RequeueAfter: r.failureBackoff()}, false
	}
	log.V(1).Info("Karpenter EC2 instances termination complete", "phase", "delete-ec2")

	// Clean up orphan ENIs created by amazon-vpc-cni that can block node SecurityGroup deletion.
	eniDone, eniDeleted, eniRemaining, err := r.cleanupOrphanCNINetworkInterfacesForNodeSecurityGroup(ctx, cluster, region)
	if err != nil {
		msg := fmt.Sprintf(
			"failed to cleanup orphan ENIs for node SecurityGroup; cause: SG deletion may be stuck with DependencyViolation. action: ensure bootstrapper has ec2:DescribeNetworkInterfaces/ec2:DeleteNetworkInterface, or follow break-glass docs. error=%v",
			err,
		)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonOrphanENICleanup, msg)
		log.Error(err, "failed to cleanup orphan CNI ENIs", "deletedENIs", eniDeleted, "remainingENIs", eniRemaining)
		return ctrl.Result{RequeueAfter: r.failureBackoff()}, false
	}
	if !eniDone {
		log.V(1).Info("orphan CNI ENI cleanup in progress", "deletedENIs", eniDeleted, "remainingENIs", eniRemaining)
		if eniDeleted > 0 {
			msg := fmt.Sprintf("deleted %d orphan ENIs (remaining=%d); waiting for ENI disappearance", eniDeleted, eniRemaining)
			r.emitEvent(cluster, corev1.EventTypeNormal, reasonOrphanENICleanup, msg)
		} else {
			msg := fmt.Sprintf("orphan ENIs still present (count=%d); waiting for ENI cleanup", eniRemaining)
			r.emitEvent(cluster, corev1.EventTypeNormal, reasonOrphanENICleanup, msg)
		}
		return ctrl.Result{RequeueAfter: r.failureBackoff()}, false
	}
	if eniDeleted > 0 {
		msg := fmt.Sprintf("deleted %d orphan ENIs for node SecurityGroup", eniDeleted)
		r.emitEvent(cluster, corev1.EventTypeNormal, reasonOrphanENICleanup, msg)
	}
	log.V(1).Info("orphan CNI ENI cleanup complete", "deletedENIs", eniDeleted)
	return ctrl.Result{}, true
}

func (r *EKSKarpenterBootstrapperReconciler) suspendFluxKarpenterOnDelete(ctx context.Context, owner *clusterv1.Cluster, capiClusterName string) (bool, error) {
	if r == nil || owner == nil {
		return false, nil
	}

	changed := false
	resources := []struct {
		gvk  schema.GroupVersionKind
		name string
	}{
		{gvk: fluxOCIRepositoryGVK, name: fmt.Sprintf("%s-karpenter", capiClusterName)},
		{gvk: fluxHelmReleaseGVK, name: fmt.Sprintf("%s-karpenter", capiClusterName)},
	}

	for _, resource := range resources {
		suspended, err := r.suspendFluxResource(ctx, owner.Namespace, resource.name, resource.gvk)
		if err != nil {
			return false, err
		}
		if suspended {
			changed = true
		}
	}
	return changed, nil
}

func (r *EKSKarpenterBootstrapperReconciler) suspendFluxResource(ctx context.Context, namespace, name string, gvk schema.GroupVersionKind) (bool, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, obj); err != nil {
		if apierrors.IsNotFound(err) || isNoMatchError(err) {
			return false, nil
		}
		return false, err
	}
	if !isManagedByBootstrapper(obj.GetAnnotations()) {
		return false, nil
	}
	if suspended, found, err := unstructured.NestedBool(obj.Object, "spec", "suspend"); err == nil && found && suspended {
		return false, nil
	}

	before := obj.DeepCopy()
	mustSetNestedField(obj, true, "spec", "suspend")
	if err := popMutationError(obj); err != nil {
		return false, err
	}
	if equality.Semantic.DeepEqual(before.Object, obj.Object) {
		return false, nil
	}
	if err := r.Patch(ctx, obj, client.MergeFrom(before)); err != nil {
		return false, err
	}
	return true, nil
}

func (r *EKSKarpenterBootstrapperReconciler) readWorkloadKubeconfigMetadata(ctx context.Context, namespace, capiClusterName string) (region string, eksClusterName string, ok bool, _ error) {
	if r == nil {
		return "", "", false, nil
	}
	secretName, err := kubeconfig.SecretName(capiClusterName)
	if err != nil {
		return "", "", false, err
	}
	secret := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: secretName}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return "", "", false, nil
		}
		return "", "", false, err
	}

	ann := secret.GetAnnotations()
	if len(ann) == 0 {
		return "", "", true, nil
	}
	return strings.TrimSpace(ann[coreeks.RegionAnnotationKey]), strings.TrimSpace(ann[coreeks.EKSClusterNameAnnotationKey]), true, nil
}

func cleanupKarpenterEC2Instances(ctx context.Context, region, eksClusterName string) (done bool, toTerminateCount int, shuttingDownCount int, retErr error) {
	region = strings.TrimSpace(region)
	eksClusterName = strings.TrimSpace(eksClusterName)
	if region == "" {
		return false, 0, 0, fmt.Errorf("region is empty")
	}
	if eksClusterName == "" {
		return false, 0, 0, fmt.Errorf("eksClusterName is empty")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return false, 0, 0, fmt.Errorf("load AWS config: %w", err)
	}
	ec2c := ec2.NewFromConfig(cfg)

	nameTag := "tag:karpenter.sh/discovery"
	stateName := "instance-state-name"
	input := &ec2.DescribeInstancesInput{Filters: []ec2types.Filter{
		{Name: &nameTag, Values: []string{eksClusterName}},
		{Name: &stateName, Values: []string{"pending", "running", "stopping", "stopped", "shutting-down"}},
	}}

	toTerminate := []string{}
	stillShuttingDown := []string{}

	p := ec2.NewDescribeInstancesPaginator(ec2c, input)
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return false, 0, 0, fmt.Errorf("describe instances: %w", err)
		}
		for _, res := range page.Reservations {
			for _, inst := range res.Instances {
				id := strings.TrimSpace(derefString(inst.InstanceId))
				if id == "" {
					continue
				}
				s := string(inst.State.Name)
				switch s {
				case "pending", "running", "stopping", "stopped":
					toTerminate = append(toTerminate, id)
				case "shutting-down":
					stillShuttingDown = append(stillShuttingDown, id)
				}
			}
		}
	}

	if len(toTerminate) == 0 && len(stillShuttingDown) == 0 {
		return true, 0, 0, nil
	}

	// Terminate instances that are not already shutting down.
	if len(toTerminate) > 0 {
		const batchSize = 50
		for i := 0; i < len(toTerminate); i += batchSize {
			end := min(i+batchSize, len(toTerminate))
			batch := toTerminate[i:end]
			_, err := ec2c.TerminateInstances(ctx, &ec2.TerminateInstancesInput{InstanceIds: batch})
			if err != nil {
				if ignoreInvalidInstanceNotFound(err) {
					continue
				}
				return false, 0, 0, fmt.Errorf("terminate instances: %w", err)
			}
		}
	}

	// Cleanup is in progress.
	return false, len(toTerminate), len(stillShuttingDown), nil
}

func ignoreInvalidInstanceNotFound(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode() == "InvalidInstanceID.NotFound"
	}
	return false
}

func (r *EKSKarpenterBootstrapperReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.TokenGenerator == nil {
		generator, err := coreeks.NewAWSIAMAuthenticatorTokenGenerator()
		if err != nil {
			return err
		}
		r.TokenGenerator = generator
	}
	if r.Now == nil {
		r.Now = time.Now
	}
	if r.FailureBackoff == 0 {
		r.FailureBackoff = 30 * time.Second
	}
	if r.SteadyStateRequeue == 0 {
		r.SteadyStateRequeue = 10 * time.Minute
	}
	if r.RESTMapper == nil {
		r.RESTMapper = mgr.GetRESTMapper()
	}
	if err := ensureACKClusterNameIndex(context.Background(), mgr); err != nil {
		return err
	}

	ackCluster := &unstructured.Unstructured{}
	ackCluster.SetGroupVersionKind(ackClusterGVK)

	controllerBuilder := ctrl.NewControllerManagedBy(mgr).For(&clusterv1.Cluster{})
	if r.isAPIAvailable(ackClusterGVK) {
		controllerBuilder = controllerBuilder.Watches(ackCluster, handler.EnqueueRequestsFromMapFunc(r.mapACKClusterToCAPIClustersForKarpenter))
	} else {
		logf.Log.WithName("setup").Info(
			"skip ACK watch; API is not available",
			"controller", "eks-karpenter-bootstrapper",
			"gvk", ackClusterGVK.String(),
		)
	}

	for _, gvk := range []schema.GroupVersionKind{
		ackFargateProfileGVK,
		ackAccessEntryGVK,
		ackSecurityGroupGVK,
		ackIAMPolicyGVK,
		ackIAMRoleGVK,
		ackIAMInstanceProfileGVK,
		ackOIDCProviderGVK,
		fluxOCIRepositoryGVK,
		fluxHelmReleaseGVK,
		clusterResourceSetGVK,
	} {
		controllerBuilder = r.withOptionalWatch(mgr, controllerBuilder, gvk, r.mapManagedObjectToCAPICluster)
	}

	return controllerBuilder.
		Watches(&corev1.ConfigMap{}, handler.EnqueueRequestsFromMapFunc(r.mapManagedObjectToCAPICluster)).
		Named("eks-karpenter-bootstrapper").
		Complete(r)
}

func (r *EKSKarpenterBootstrapperReconciler) mapACKClusterToCAPIClustersForKarpenter(ctx context.Context, obj client.Object) []reconcile.Request {
	namespace := obj.GetNamespace()
	ackName := obj.GetName()
	if strings.TrimSpace(ackName) == "" {
		return nil
	}

	clusters := &clusterv1.ClusterList{}
	listOpts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingFields{ackClusterNameIndexKey: ackName},
	}
	if err := r.List(ctx, clusters, listOpts...); err != nil {
		log := logf.FromContext(ctx).WithValues("namespace", namespace, "ackClusterName", ackName)
		log.V(1).Info("ACK cluster index lookup failed; falling back to namespace list", "error", err.Error())
		clusters = &clusterv1.ClusterList{}
		if err := r.List(ctx, clusters, client.InNamespace(namespace)); err != nil {
			log.Error(err, "list CAPI clusters for ACK mapping")
			return nil
		}
	}

	requests := []reconcile.Request{}
	seen := map[client.ObjectKey]struct{}{}
	for i := range clusters.Items {
		cluster := &clusters.Items[i]
		if !isKarpenterEnabled(cluster) {
			continue
		}
		_, _, resolvedAckName := resolveClusterNames(cluster)
		if resolvedAckName != ackName {
			continue
		}
		key := client.ObjectKey{Namespace: cluster.Namespace, Name: cluster.Name}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		requests = append(requests, reconcile.Request{NamespacedName: key})
	}
	return requests
}

func (r *EKSKarpenterBootstrapperReconciler) mapManagedObjectToCAPICluster(_ context.Context, obj client.Object) []reconcile.Request {
	if obj == nil {
		return nil
	}
	labels := obj.GetLabels()
	if len(labels) == 0 {
		return nil
	}
	clusterName := strings.TrimSpace(labels[capiClusterNameLabelKey])
	if clusterName == "" {
		return nil
	}
	if !isManagedByBootstrapper(obj.GetAnnotations()) {
		return nil
	}
	namespace := strings.TrimSpace(obj.GetNamespace())
	if namespace == "" {
		return nil
	}
	return []reconcile.Request{{
		NamespacedName: client.ObjectKey{
			Namespace: namespace,
			Name:      clusterName,
		},
	}}
}

func (r *EKSKarpenterBootstrapperReconciler) withOptionalWatch(
	_ ctrl.Manager,
	b *builder.Builder,
	gvk schema.GroupVersionKind,
	mapFn handler.MapFunc,
) *builder.Builder {
	if !r.isAPIAvailable(gvk) {
		logf.Log.WithName("setup").Info(
			"skip optional watch; API is not available",
			"controller", "eks-karpenter-bootstrapper",
			"gvk", gvk.String(),
		)
		return b
	}
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	return b.Watches(obj, handler.EnqueueRequestsFromMapFunc(mapFn))
}

func isKarpenterEnabled(cluster *clusterv1.Cluster) bool {
	if cluster == nil || cluster.Labels == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(cluster.Labels[karpenterEnableLabelKey]), karpenterEnableLabelValue)
}

func (r *EKSKarpenterBootstrapperReconciler) ensureClusterNameLabel(ctx context.Context, cluster *clusterv1.Cluster) error {
	if r == nil || cluster == nil {
		return nil
	}
	if cluster.Labels != nil {
		if v := strings.TrimSpace(cluster.Labels[capiClusterNameLabelKey]); v == cluster.Name {
			return nil
		}
	}

	before := cluster.DeepCopy()
	if cluster.Labels == nil {
		cluster.Labels = map[string]string{}
	}
	cluster.Labels[capiClusterNameLabelKey] = cluster.Name
	if equality.Semantic.DeepEqual(before.Labels, cluster.Labels) {
		return nil
	}
	return r.Patch(ctx, cluster, client.MergeFrom(before))
}

func (r *EKSKarpenterBootstrapperReconciler) ensureCleanupFinalizer(ctx context.Context, cluster *clusterv1.Cluster) error {
	if r == nil || cluster == nil {
		return nil
	}
	before := cluster.DeepCopy()
	controllerutil.AddFinalizer(cluster, r.cleanupFinalizer())
	if equality.Semantic.DeepEqual(before.Finalizers, cluster.Finalizers) {
		return nil
	}
	return r.Patch(ctx, cluster, client.MergeFrom(before))
}

func (r *EKSKarpenterBootstrapperReconciler) removeCleanupFinalizer(ctx context.Context, cluster *clusterv1.Cluster) error {
	if r == nil || cluster == nil {
		return nil
	}
	before := cluster.DeepCopy()
	controllerutil.RemoveFinalizer(cluster, r.cleanupFinalizer())
	if equality.Semantic.DeepEqual(before.Finalizers, cluster.Finalizers) {
		return nil
	}
	return r.Patch(ctx, cluster, client.MergeFrom(before))
}

func (r *EKSKarpenterBootstrapperReconciler) cleanupFinalizer() string {
	if r == nil {
		return karpenterCleanupFinalizer
	}
	if v := strings.TrimSpace(r.CleanupFinalizer); v != "" {
		return v
	}
	return karpenterCleanupFinalizer
}

func (r *EKSKarpenterBootstrapperReconciler) resolveKarpenterChartTag(cluster *clusterv1.Cluster) string {
	if cluster != nil && cluster.Annotations != nil {
		if v := strings.TrimSpace(cluster.Annotations[karpenterChartVersionAnnotation]); v != "" {
			return v
		}
	}
	if r != nil {
		if v := strings.TrimSpace(r.KarpenterChartTag); v != "" {
			return v
		}
	}
	return defaultKarpenterChartVersion
}

func (r *EKSKarpenterBootstrapperReconciler) resolveKarpenterHelmValues(
	owner *clusterv1.Cluster,
	eksClusterName,
	endpoint,
	controllerRoleARN string,
) (map[string]any, error) {
	values := defaultKarpenterHelmValues(eksClusterName, endpoint, controllerRoleARN)
	if owner == nil || len(owner.Annotations) == 0 {
		return values, nil
	}

	if interruptionQueue := strings.TrimSpace(owner.Annotations[karpenterInterruptionQueueAnnotation]); interruptionQueue != "" {
		mustEnsureNestedMap(values, "settings")["interruptionQueue"] = interruptionQueue
	}

	rawOverride := strings.TrimSpace(owner.Annotations[karpenterHelmValuesAnnotation])
	if rawOverride == "" {
		return values, nil
	}

	override := map[string]any{}
	if err := json.Unmarshal([]byte(rawOverride), &override); err != nil {
		return nil, fmt.Errorf("unmarshal JSON override: %w", err)
	}
	values = deepMergeMapAny(values, override)

	// Keep critical keys owned by the bootstrapper even when an override is provided.
	mustEnsureNestedMap(values, "settings")["clusterName"] = eksClusterName
	mustEnsureNestedMap(values, "settings")["clusterEndpoint"] = endpoint
	mustEnsureNestedMap(mustEnsureNestedMap(values, "serviceAccount"), "annotations")["eks.amazonaws.com/role-arn"] = controllerRoleARN
	return values, nil
}

func defaultKarpenterHelmValues(eksClusterName, endpoint, controllerRoleARN string) map[string]any {
	return map[string]any{
		"dnsPolicy":         "Default",
		"priorityClassName": "system-cluster-critical",
		"webhook":           map[string]any{"enabled": false},
		"settings":          map[string]any{"clusterName": eksClusterName, "clusterEndpoint": endpoint},
		"serviceAccount":    map[string]any{"annotations": map[string]any{"eks.amazonaws.com/role-arn": controllerRoleARN}},
	}
}

func (r *EKSKarpenterBootstrapperReconciler) resolveNodePoolTemplateYAML(
	ctx context.Context,
	owner *clusterv1.Cluster,
	eksClusterName,
	nodeInstanceProfileName string,
	nodeSubnetIDs,
	securityGroupIDs []string,
) (string, error) {
	defaultYAML := buildDefaultNodePoolYAML(eksClusterName, nodeInstanceProfileName, nodeSubnetIDs, securityGroupIDs)
	if owner == nil || len(owner.Annotations) == 0 {
		return defaultYAML, nil
	}

	cmName := strings.TrimSpace(owner.Annotations[karpenterNodePoolTemplateConfigMapAnnotation])
	if cmName == "" {
		return defaultYAML, nil
	}
	key := strings.TrimSpace(owner.Annotations[karpenterNodePoolTemplateConfigMapKeyAnnotation])
	if key == "" {
		key = "resources.yaml"
	}

	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: owner.Namespace, Name: cmName}, cm); err != nil {
		return "", fmt.Errorf("%w: read ConfigMap %s/%s: %v", errNodePoolTemplateInvalid, owner.Namespace, cmName, err)
	}
	raw := strings.TrimSpace(cm.Data[key])
	if raw == "" {
		return "", fmt.Errorf("%w: ConfigMap %s/%s has empty data[%q]", errNodePoolTemplateInvalid, owner.Namespace, cmName, key)
	}
	return raw + "\n", nil
}

func (r *EKSKarpenterBootstrapperReconciler) ensureOIDCProvider(ctx context.Context, owner *clusterv1.Cluster, name, region, issuerURL string, thumbprints []string) (bool, error) {
	obj := newUnstructured(ackOIDCProviderGVK, owner.Namespace, name)
	return r.upsertManagedUnstructured(ctx, owner, obj, func(u *unstructured.Unstructured) error {
		setRegionAnnotation(u, region)
		setManagedBy(u)
		setClusterLabel(u, owner.Name)
		mustSetNestedString(u, issuerURL, "spec", "url")
		// ACK IAM OpenIDConnectProvider uses spec.clientIDs.
		mustSetNestedSlice(u, []any{"sts.amazonaws.com"}, "spec", "clientIDs")
		if len(thumbprints) > 0 {
			values := make([]any, 0, len(thumbprints))
			for _, thumb := range thumbprints {
				thumb = strings.TrimSpace(thumb)
				if thumb == "" {
					continue
				}
				values = append(values, thumb)
			}
			if len(values) > 0 {
				mustSetNestedSlice(u, values, "spec", "thumbprints")
			}
		}
		mustSetNestedSlice(u, awsTagsAsSlice(bootstrapperResourceTags(owner, nil)), "spec", "tags")
		// thumbprints are optional; IAM can retrieve them automatically.
		return nil
	})
}

func (r *EKSKarpenterBootstrapperReconciler) ensureIAMPolicy(ctx context.Context, owner *clusterv1.Cluster, name, region, policyDocument, policyName string) (bool, error) {
	obj := newUnstructured(ackIAMPolicyGVK, owner.Namespace, name)
	return r.upsertManagedUnstructured(ctx, owner, obj, func(u *unstructured.Unstructured) error {
		setRegionAnnotation(u, region)
		setManagedBy(u)
		setClusterLabel(u, owner.Name)
		mustSetNestedString(u, policyName, "spec", "name")
		mustSetNestedString(u, policyDocument, "spec", "policyDocument")
		mustSetNestedSlice(u, awsTagsAsSlice(bootstrapperResourceTags(owner, nil)), "spec", "tags")
		return nil
	})
}

func (r *EKSKarpenterBootstrapperReconciler) ensureIAMRoleForIRSA(ctx context.Context, owner *clusterv1.Cluster, name, region, awsRoleName, assumeRolePolicyDocument string, policyRefNames []string) (bool, error) {
	obj := newUnstructured(ackIAMRoleGVK, owner.Namespace, name)
	return r.upsertManagedUnstructured(ctx, owner, obj, func(u *unstructured.Unstructured) error {
		setRegionAnnotation(u, region)
		setManagedBy(u)
		setClusterLabel(u, owner.Name)
		mustSetNestedString(u, awsRoleName, "spec", "name")
		mustSetNestedString(u, assumeRolePolicyDocument, "spec", "assumeRolePolicyDocument")
		refs := []any{}
		for _, refName := range policyRefNames {
			refs = append(refs, map[string]any{"from": map[string]any{"name": refName, "namespace": owner.Namespace}})
		}
		mustSetNestedSlice(u, refs, "spec", "policyRefs")
		mustSetNestedSlice(u, awsTagsAsSlice(bootstrapperResourceTags(owner, nil)), "spec", "tags")
		return nil
	})
}

func (r *EKSKarpenterBootstrapperReconciler) ensureIAMRoleForEC2(ctx context.Context, owner *clusterv1.Cluster, name, region, awsRoleName string, managedPolicyARNs []string) (bool, error) {
	obj := newUnstructured(ackIAMRoleGVK, owner.Namespace, name)
	assume := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	return r.upsertManagedUnstructured(ctx, owner, obj, func(u *unstructured.Unstructured) error {
		setRegionAnnotation(u, region)
		setManagedBy(u)
		setClusterLabel(u, owner.Name)
		mustSetNestedString(u, awsRoleName, "spec", "name")
		mustSetNestedString(u, assume, "spec", "assumeRolePolicyDocument")
		policies := []any{}
		for _, arn := range managedPolicyARNs {
			policies = append(policies, arn)
		}
		mustSetNestedSlice(u, policies, "spec", "policies")
		mustSetNestedSlice(u, awsTagsAsSlice(bootstrapperResourceTags(owner, nil)), "spec", "tags")
		return nil
	})
}

func (r *EKSKarpenterBootstrapperReconciler) ensureIAMInstanceProfile(ctx context.Context, owner *clusterv1.Cluster, name, region, awsInstanceProfileName, roleRefName string) (bool, error) {
	obj := newUnstructured(ackIAMInstanceProfileGVK, owner.Namespace, name)
	return r.upsertManagedUnstructured(ctx, owner, obj, func(u *unstructured.Unstructured) error {
		setRegionAnnotation(u, region)
		setManagedBy(u)
		setClusterLabel(u, owner.Name)
		mustSetNestedString(u, awsInstanceProfileName, "spec", "name")
		mustSetNestedField(u, map[string]any{"from": map[string]any{"name": roleRefName, "namespace": owner.Namespace}}, "spec", "roleRef")
		mustSetNestedSlice(u, awsTagsAsSlice(bootstrapperResourceTags(owner, nil)), "spec", "tags")
		return nil
	})
}

func (r *EKSKarpenterBootstrapperReconciler) ensureAccessEntry(ctx context.Context, owner *clusterv1.Cluster, name, region, ackClusterName, principalARN string) (bool, error) {
	obj := newUnstructured(ackAccessEntryGVK, owner.Namespace, name)
	return r.upsertManagedUnstructured(ctx, owner, obj, func(u *unstructured.Unstructured) error {
		setRegionAnnotation(u, region)
		setManagedBy(u)
		setClusterLabel(u, owner.Name)
		mustSetNestedString(u, principalARN, "spec", "principalARN")
		mustSetNestedString(u, "EC2_LINUX", "spec", "type")
		mustSetNestedField(u, map[string]any{"from": map[string]any{"name": ackClusterName, "namespace": owner.Namespace}}, "spec", "clusterRef")
		mustSetNestedField(u, awsTagsAsMap(bootstrapperResourceTags(owner, nil)), "spec", "tags")
		return nil
	})
}

func (r *EKSKarpenterBootstrapperReconciler) ensureNodeSecurityGroup(ctx context.Context, owner *clusterv1.Cluster, region, eksClusterName string, nodeSubnetIDs []string) (string, bool, error) {
	if owner == nil {
		return "", false, fmt.Errorf("owner cluster is nil")
	}
	region = strings.TrimSpace(region)
	if region == "" {
		return "", false, fmt.Errorf("region is empty")
	}
	eksClusterName = strings.TrimSpace(eksClusterName)
	if eksClusterName == "" {
		return "", false, fmt.Errorf("eks cluster name is empty")
	}
	if len(nodeSubnetIDs) == 0 {
		return "", false, fmt.Errorf("subnet IDs are empty")
	}

	vpcID, vpcCIDRs, err := discoverVPCBySubnets(ctx, region, nodeSubnetIDs)
	if err != nil {
		return "", false, err
	}
	if vpcID == "" {
		return "", false, fmt.Errorf("resolved vpc ID is empty")
	}
	if len(vpcCIDRs) == 0 {
		return "", false, fmt.Errorf("resolved VPC CIDRs are empty")
	}

	objName := shortenAWSName(fmt.Sprintf("%s-karpenter-node-sg", owner.Name), 63)
	awsName := shortenAWSName(fmt.Sprintf("%s-karpenter-node", eksClusterName), 255)
	desc := fmt.Sprintf("Karpenter nodes for %s", eksClusterName)
	if len(desc) > 255 {
		desc = desc[:255]
	}

	obj := newUnstructured(ackSecurityGroupGVK, owner.Namespace, objName)
	managed, err := r.upsertManagedUnstructured(ctx, owner, obj, func(u *unstructured.Unstructured) error {
		setRegionAnnotation(u, region)
		setManagedBy(u)
		setClusterLabel(u, owner.Name)

		mustSetNestedString(u, awsName, "spec", "name")
		mustSetNestedString(u, desc, "spec", "description")
		mustSetNestedString(u, vpcID, "spec", "vpcID")

		// Ingress: allow all traffic from within the VPC CIDR(s).
		ipRanges := []any{}
		for _, c := range vpcCIDRs {
			ipRanges = append(ipRanges, map[string]any{"cidrIP": c, "description": "VPC traffic"})
		}
		mustSetNestedSlice(u, []any{map[string]any{"ipProtocol": "-1", "ipRanges": ipRanges}}, "spec", "ingressRules")

		// Egress: explicit allow-all.
		mustSetNestedSlice(u, []any{map[string]any{"ipProtocol": "-1", "ipRanges": []any{map[string]any{"cidrIP": "0.0.0.0/0"}}}}, "spec", "egressRules")

		// Tags help with discovery/debugging.
		tags := bootstrapperResourceTags(owner, map[string]string{
			"Name":                   awsName,
			"karpenter.sh/discovery": eksClusterName,
			fmt.Sprintf("kubernetes.io/cluster/%s", eksClusterName): "owned",
		})
		mustSetNestedSlice(u, awsTagsAsSlice(tags), "spec", "tags")
		return nil
	})
	if err != nil {
		if isNoMatchError(err) {
			return "", true, fmt.Errorf("ACK EC2 SecurityGroup CRD is not installed")
		}
		return "", false, err
	}
	if !managed {
		return "", false, nil
	}

	// Read status.id (security group ID) from the object.
	got := &unstructured.Unstructured{}
	got.SetGroupVersionKind(ackSecurityGroupGVK)
	if err := r.Get(ctx, client.ObjectKey{Namespace: owner.Namespace, Name: objName}, got); err != nil {
		return "", true, err
	}
	id, _ := readNestedString(got.Object, "status", "id")
	return strings.TrimSpace(id), true, nil
}

func discoverVPCBySubnets(ctx context.Context, region string, nodeSubnetIDs []string) (string, []string, error) {
	region = strings.TrimSpace(region)
	if region == "" {
		return "", nil, fmt.Errorf("region is empty")
	}
	ids := []string{}
	for _, id := range nodeSubnetIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return "", nil, fmt.Errorf("subnet IDs are empty")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return "", nil, fmt.Errorf("load AWS config: %w", err)
	}
	ec2c := ec2.NewFromConfig(cfg)

	subOut, err := ec2c.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{SubnetIds: ids})
	if err != nil {
		return "", nil, fmt.Errorf("describe subnets: %w", err)
	}
	if len(subOut.Subnets) == 0 {
		return "", nil, fmt.Errorf("no subnets returned for %v", ids)
	}

	vpcID := strings.TrimSpace(derefString(subOut.Subnets[0].VpcId))
	if vpcID == "" {
		return "", nil, fmt.Errorf("subnet has empty vpcId")
	}
	for _, s := range subOut.Subnets {
		if got := strings.TrimSpace(derefString(s.VpcId)); got != vpcID {
			return "", nil, fmt.Errorf("subnets are in different VPCs (got %q and %q)", vpcID, got)
		}
	}

	vpcOut, err := ec2c.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{VpcIds: []string{vpcID}})
	if err != nil {
		return "", nil, fmt.Errorf("describe vpcs: %w", err)
	}
	if len(vpcOut.Vpcs) == 0 {
		return "", nil, fmt.Errorf("no VPC returned for %s", vpcID)
	}
	v := vpcOut.Vpcs[0]

	set := map[string]struct{}{}
	add := func(c string) {
		c = strings.TrimSpace(c)
		if c == "" {
			return
		}
		set[c] = struct{}{}
	}
	add(derefString(v.CidrBlock))
	for _, a := range v.CidrBlockAssociationSet {
		add(derefString(a.CidrBlock))
	}

	cidrs := make([]string, 0, len(set))
	for c := range set {
		cidrs = append(cidrs, c)
	}
	sort.Strings(cidrs)
	return vpcID, cidrs, nil
}

type fargateSubnetValidationResult struct {
	SubnetsWithoutNATDefaultRoute []string
	DistinctAZCount               int
}

// validateFargateSubnets orchestrates a series of AWS SDK calls to inspect
// each candidate subnet; the branching reflects the number of distinct
// failure modes reported to Fargate operators, so cyclomatic complexity is
// acceptable here.
//
// This validator runs against node subnets only (vpc-node-subnet-ids), since
// EKS FargateProfile and the default EC2NodeClass are the consumers of these
// subnets. Hard failures returned as error: empty input, AWS API errors,
// unknown subnet IDs, multiple VPCs, and public subnets. NAT default route
// is checked but surfaced as a warning via SubnetsWithoutNATDefaultRoute
// (image pulls may fail without it but it is not a hard AWS requirement;
// VPC endpoints can substitute). AZ diversity is reported via
// DistinctAZCount; the caller emits a non-blocking warning when
// DistinctAZCount < 2 (HA recommendation, not enforced). Control plane
// subnet IDs are consumed directly by EKS resourcesVPCConfig.subnetIDs and
// are not validated here.
//
//nolint:gocyclo // sequential AWS probe with per-check error surfacing
func validateFargateSubnets(ctx context.Context, region string, nodeSubnetIDs []string) (fargateSubnetValidationResult, error) {
	ids := normalizeDistinctStrings(nodeSubnetIDs)
	if len(ids) == 0 {
		return fargateSubnetValidationResult{}, fmt.Errorf("need at least 1 subnet, got 0")
	}
	region = strings.TrimSpace(region)
	if region == "" {
		return fargateSubnetValidationResult{}, fmt.Errorf("region is empty")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return fargateSubnetValidationResult{}, fmt.Errorf("load AWS config: %w", err)
	}
	ec2c := ec2.NewFromConfig(cfg)

	subOut, err := ec2c.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{SubnetIds: ids})
	if err != nil {
		return fargateSubnetValidationResult{}, fmt.Errorf("describe subnets: %w", err)
	}
	if len(subOut.Subnets) == 0 {
		return fargateSubnetValidationResult{}, fmt.Errorf("no subnets returned")
	}

	subnetByID := map[string]ec2types.Subnet{}
	for _, s := range subOut.Subnets {
		id := strings.TrimSpace(derefString(s.SubnetId))
		if id == "" {
			continue
		}
		subnetByID[id] = s
	}
	for _, id := range ids {
		if _, ok := subnetByID[id]; !ok {
			return fargateSubnetValidationResult{}, fmt.Errorf("subnet %q not found", id)
		}
	}

	vpcID := ""
	azSet := map[string]struct{}{}
	for _, id := range ids {
		s := subnetByID[id]
		gotVPCID := strings.TrimSpace(derefString(s.VpcId))
		if gotVPCID == "" {
			return fargateSubnetValidationResult{}, fmt.Errorf("subnet %q has empty vpcId", id)
		}
		if vpcID == "" {
			vpcID = gotVPCID
		} else if vpcID != gotVPCID {
			return fargateSubnetValidationResult{}, fmt.Errorf("subnets are in different VPCs (%q, %q)", vpcID, gotVPCID)
		}
		az := strings.TrimSpace(derefString(s.AvailabilityZone))
		if az != "" {
			azSet[az] = struct{}{}
		}
	}
	// AZ count is non-blocking: AWS FargateProfile accepts >=1 subnet (no AZ
	// minimum). HA across >=2 AZs is recommended but not required; the caller
	// emits a warning event when DistinctAZCount < 2.

	rtOut, err := ec2c.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{
		Filters: []ec2types.Filter{
			{
				Name:   stringPtr("association.subnet-id"),
				Values: ids,
			},
		},
	})
	if err != nil {
		return fargateSubnetValidationResult{}, fmt.Errorf("describe route tables for subnet associations: %w", err)
	}

	routeTableBySubnet := map[string]ec2types.RouteTable{}
	for _, rt := range rtOut.RouteTables {
		for _, assoc := range rt.Associations {
			subnetID := strings.TrimSpace(derefString(assoc.SubnetId))
			if subnetID == "" {
				continue
			}
			routeTableBySubnet[subnetID] = rt
		}
	}

	var mainRouteTable *ec2types.RouteTable
	if len(routeTableBySubnet) < len(ids) {
		rtAll, err := ec2c.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{
			Filters: []ec2types.Filter{
				{
					Name:   stringPtr("vpc-id"),
					Values: []string{vpcID},
				},
			},
		})
		if err != nil {
			return fargateSubnetValidationResult{}, fmt.Errorf("describe route tables in VPC %s: %w", vpcID, err)
		}
		for i := range rtAll.RouteTables {
			rt := rtAll.RouteTables[i]
			for _, assoc := range rt.Associations {
				if assoc.Main != nil && *assoc.Main {
					mainRouteTable = &rtAll.RouteTables[i]
					break
				}
			}
			if mainRouteTable != nil {
				break
			}
		}
	}

	publicSubnets := []string{}
	subnetsWithoutNATDefaultRoute := []string{}
	for _, subnetID := range ids {
		rt, found := routeTableBySubnet[subnetID]
		if !found {
			if mainRouteTable == nil {
				return fargateSubnetValidationResult{}, fmt.Errorf("route table not found for subnet %q and no main route table detected", subnetID)
			}
			rt = *mainRouteTable
		}

		hasIGWDefaultRoute := false
		hasNATDefaultRoute := false
		for _, route := range rt.Routes {
			if !isDefaultIPv4OrIPv6Route(route) {
				continue
			}

			gw := strings.TrimSpace(derefString(route.GatewayId))
			if strings.HasPrefix(gw, "igw-") {
				hasIGWDefaultRoute = true
			}
			if strings.HasPrefix(strings.TrimSpace(derefString(route.NatGatewayId)), "nat-") || strings.TrimSpace(derefString(route.InstanceId)) != "" {
				hasNATDefaultRoute = true
			}
		}

		if hasIGWDefaultRoute {
			publicSubnets = append(publicSubnets, subnetID)
		}
		if !hasNATDefaultRoute {
			subnetsWithoutNATDefaultRoute = append(subnetsWithoutNATDefaultRoute, subnetID)
		}
	}
	if len(publicSubnets) > 0 {
		sort.Strings(publicSubnets)
		return fargateSubnetValidationResult{}, fmt.Errorf("public subnet IDs are not allowed for FargateProfile: %v", publicSubnets)
	}
	sort.Strings(subnetsWithoutNATDefaultRoute)
	return fargateSubnetValidationResult{
		SubnetsWithoutNATDefaultRoute: subnetsWithoutNATDefaultRoute,
		DistinctAZCount:               len(azSet),
	}, nil
}

func isDefaultIPv4OrIPv6Route(route ec2types.Route) bool {
	dest4 := strings.TrimSpace(derefString(route.DestinationCidrBlock))
	dest6 := strings.TrimSpace(derefString(route.DestinationIpv6CidrBlock))
	return dest4 == "0.0.0.0/0" || dest6 == "::/0"
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func stringPtr(v string) *string {
	return &v
}

func (r *EKSKarpenterBootstrapperReconciler) ensureFluxKarpenter(ctx context.Context, owner *clusterv1.Cluster, capiClusterName, karpenterChartTag string, helmValues map[string]any) (bool, error) {
	// OCIRepository
	ociRepoName := fmt.Sprintf("%s-karpenter", capiClusterName)
	oci := newUnstructured(fluxOCIRepositoryGVK, owner.Namespace, ociRepoName)
	if ok, err := r.upsertManagedUnstructured(ctx, owner, oci, func(u *unstructured.Unstructured) error {
		setManagedBy(u)
		setClusterLabel(u, owner.Name)
		mustSetNestedString(u, "10m", "spec", "interval")
		mustSetNestedString(u, "oci://public.ecr.aws/karpenter/karpenter", "spec", "url")
		mustSetNestedField(u, map[string]any{"tag": karpenterChartTag}, "spec", "ref")
		mustSetNestedField(u, map[string]any{
			"mediaType": "application/vnd.cncf.helm.chart.content.v1.tar+gzip",
			"operation": "copy",
		}, "spec", "layerSelector")
		return nil
	}); err != nil {
		return false, err
	} else if !ok {
		return false, nil
	}

	// HelmRelease
	hrName := fmt.Sprintf("%s-karpenter", capiClusterName)
	hr := newUnstructured(fluxHelmReleaseGVK, owner.Namespace, hrName)
	ok, err := r.upsertManagedUnstructured(ctx, owner, hr, func(u *unstructured.Unstructured) error {
		setManagedBy(u)
		setClusterLabel(u, owner.Name)
		mustSetNestedString(u, "5m", "spec", "interval")
		mustSetNestedString(u, "15m", "spec", "timeout")
		mustSetNestedString(u, "karpenter", "spec", "releaseName")
		mustSetNestedString(u, "karpenter", "spec", "targetNamespace")
		mustSetNestedField(u, map[string]any{"secretRef": map[string]any{"name": fmt.Sprintf("%s-kubeconfig", capiClusterName)}}, "spec", "kubeConfig")
		mustSetNestedField(u, map[string]any{
			"kind":      "OCIRepository",
			"name":      ociRepoName,
			"namespace": owner.Namespace,
		}, "spec", "chartRef")
		mustSetNestedField(u, map[string]any{
			"createNamespace": true,
			"crds":            "CreateReplace",
			"disableWait":     true,
			"remediation":     map[string]any{"retries": int64(-1)},
		}, "spec", "install")
		mustSetNestedField(u, map[string]any{
			"crds":        "CreateReplace",
			"disableWait": true,
			"remediation": map[string]any{"retries": int64(-1)},
		}, "spec", "upgrade")
		mustSetNestedField(u, deepCopyMapAny(helmValues), "spec", "values")
		return nil
	})
	if err != nil {
		return false, err
	}
	return ok, nil
}

func (r *EKSKarpenterBootstrapperReconciler) ensureDefaultNodePoolResources(ctx context.Context, owner *clusterv1.Cluster, capiClusterName, eksClusterName, nodeInstanceProfileName string, nodeSubnetIDs, securityGroupIDs []string) error {
	desiredYAML, err := r.resolveNodePoolTemplateYAML(ctx, owner, eksClusterName, nodeInstanceProfileName, nodeSubnetIDs, securityGroupIDs)
	if err != nil {
		return err
	}

	// 1) ConfigMap containing default EC2NodeClass + NodePool.
	cmName := fmt.Sprintf("%s-karpenter-nodepool", capiClusterName)
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: owner.Namespace}}
	if err := r.Get(ctx, client.ObjectKeyFromObject(cm), cm); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		cm = &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: owner.Namespace}}
		mutateManagedConfigMap(cm, owner, desiredYAML)
		if err := controllerutil.SetOwnerReference(owner, cm, r.Scheme); err != nil {
			return err
		}
		if err := r.Create(ctx, cm); err != nil {
			return err
		}
	} else {
		before := cm.DeepCopy()
		wasManaged := isManagedByBootstrapper(cm.GetAnnotations())
		if !wasManaged && !coreeks.IsUnmanagedTakeoverEnabled(owner.GetAnnotations()) {
			msg := fmt.Sprintf("configmap %s/%s exists and is not managed by %s", owner.Namespace, cmName, karpenterManagedByValue)
			r.emitEvent(owner, corev1.EventTypeWarning, reasonResourceOwnership, msg)
			recordOwnershipConflict(metricControllerBootstrapper, "ConfigMap")
			return nil
		}
		mutateManagedConfigMap(cm, owner, desiredYAML)
		if err := controllerutil.SetOwnerReference(owner, cm, r.Scheme); err != nil {
			return err
		}
		if !equality.Semantic.DeepEqual(before, cm) {
			if err := r.Update(ctx, cm); err != nil {
				return err
			}
		}
		if !wasManaged {
			msg := fmt.Sprintf("took over unmanaged ConfigMap %s/%s because %q is enabled", owner.Namespace, cmName, coreeks.AllowUnmanagedTakeoverAnnotationKey)
			r.emitEvent(owner, corev1.EventTypeNormal, reasonResourceTakeover, msg)
		}
	}

	// 2) ClusterResourceSet to apply the ConfigMap to the remote cluster.
	crsName := fmt.Sprintf("%s-karpenter-nodepool", capiClusterName)
	crs := newUnstructured(clusterResourceSetGVK, owner.Namespace, crsName)
	ok, err := r.upsertManagedUnstructured(ctx, owner, crs, func(u *unstructured.Unstructured) error {
		setManagedBy(u)
		setClusterLabel(u, owner.Name)
		mustSetNestedField(u, map[string]any{"matchLabels": map[string]any{capiClusterNameLabelKey: capiClusterName, karpenterEnableLabelKey: karpenterEnableLabelValue}}, "spec", "clusterSelector")
		mustSetNestedSlice(u, []any{map[string]any{"kind": "ConfigMap", "name": cmName}}, "spec", "resources")
		mustSetNestedString(u, "Reconcile", "spec", "strategy")
		return nil
	})
	if err != nil {
		if isNoMatchError(err) {
			// ClusterResourceSet API is not installed; ignore for now.
			return nil
		}
		return err
	}
	if !ok {
		msg := fmt.Sprintf("ClusterResourceSet %s/%s exists and is not managed by %s", owner.Namespace, crsName, karpenterManagedByValue)
		r.emitEvent(owner, corev1.EventTypeWarning, reasonResourceOwnership, msg)
		recordOwnershipConflict(metricControllerBootstrapper, "ClusterResourceSet")
	}
	return nil
}

func buildDefaultNodePoolYAML(eksClusterName, nodeInstanceProfileName string, nodeSubnetIDs, securityGroupIDs []string) string {
	// EC2NodeClass (v1)
	subnetTerms := []string{}
	for _, id := range nodeSubnetIDs {
		subnetTerms = append(subnetTerms, fmt.Sprintf("    - id: %s", id))
	}
	sgTerms := []string{}
	for _, id := range securityGroupIDs {
		sgTerms = append(sgTerms, fmt.Sprintf("    - id: %s", id))
	}

	return strings.TrimSpace(fmt.Sprintf(`
apiVersion: karpenter.k8s.aws/v1
kind: EC2NodeClass
metadata:
  name: default
spec:
  amiSelectorTerms:
    - alias: bottlerocket@latest
  instanceProfile: %s
  subnetSelectorTerms:
%s
  securityGroupSelectorTerms:
%s
  tags:
    karpenter.sh/discovery: %q
---
apiVersion: karpenter.sh/v1
kind: NodePool
metadata:
  name: default
spec:
  template:
    spec:
      nodeClassRef:
        group: karpenter.k8s.aws
        kind: EC2NodeClass
        name: default
      requirements:
        - key: "karpenter.k8s.aws/instance-category"
          operator: In
          values: ["c", "m", "r"]
        - key: "karpenter.k8s.aws/instance-hypervisor"
          operator: In
          values: ["nitro"]
        - key: "karpenter.k8s.aws/instance-generation"
          operator: Gt
          values: ["2"]
  limits:
    cpu: 1000
  disruption:
    consolidationPolicy: WhenEmptyOrUnderutilized
    consolidateAfter: 1m
`, nodeInstanceProfileName, strings.Join(subnetTerms, "\n"), strings.Join(sgTerms, "\n"), eksClusterName)) + "\n"
}

func mutateManagedConfigMap(cm *corev1.ConfigMap, owner *clusterv1.Cluster, resourcesYAML string) {
	if cm.Labels == nil {
		cm.Labels = map[string]string{}
	}
	cm.Labels[capiClusterNameLabelKey] = owner.Name

	if cm.Annotations == nil {
		cm.Annotations = map[string]string{}
	}
	cm.Annotations[coreeks.ManagedByAnnotationKey] = karpenterManagedByValue

	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data["resources.yaml"] = resourcesYAML
}

func normalizeDistinctStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func bootstrapperResourceTags(owner *clusterv1.Cluster, extra map[string]string) map[string]string {
	tags := map[string]string{
		"kany8s.io/managed-by":        karpenterManagedByValue,
		"kany8s.io/cluster-name":      "",
		"kany8s.io/cluster-namespace": "",
	}
	if owner != nil {
		tags["kany8s.io/cluster-name"] = owner.Name
		tags["kany8s.io/cluster-namespace"] = owner.Namespace
		if owner.Annotations != nil {
			if v := strings.TrimSpace(owner.Annotations[coreeks.EKSClusterNameAnnotationKey]); v != "" {
				tags["kany8s.io/eks-cluster-name"] = v
			}
		}
	}
	for k, v := range extra {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		tags[key] = strings.TrimSpace(v)
	}
	for k, v := range tags {
		if strings.TrimSpace(v) != "" {
			continue
		}
		delete(tags, k)
	}
	return tags
}

func awsTagsAsSlice(tags map[string]string) []any {
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]any, 0, len(keys))
	for _, key := range keys {
		out = append(out, map[string]any{
			"key":   key,
			"value": tags[key],
		})
	}
	return out
}

func awsTagsAsMap(tags map[string]string) map[string]any {
	out := map[string]any{}
	for key, value := range tags {
		out[key] = value
	}
	return out
}

func deepCopyMapAny(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := map[string]any{}
	for key, value := range in {
		switch typed := value.(type) {
		case map[string]any:
			out[key] = deepCopyMapAny(typed)
		default:
			out[key] = typed
		}
	}
	return out
}

func deepMergeMapAny(base map[string]any, override map[string]any) map[string]any {
	merged := deepCopyMapAny(base)
	for key, value := range override {
		existing, found := merged[key]
		overrideMap, overrideIsMap := value.(map[string]any)
		existingMap, existingIsMap := existing.(map[string]any)
		if found && overrideIsMap && existingIsMap {
			merged[key] = deepMergeMapAny(existingMap, overrideMap)
			continue
		}
		if overrideIsMap {
			merged[key] = deepCopyMapAny(overrideMap)
			continue
		}
		merged[key] = value
	}
	return merged
}

func mustEnsureNestedMap(root map[string]any, key string) map[string]any {
	key = strings.TrimSpace(key)
	if key == "" {
		return root
	}
	if root == nil {
		root = map[string]any{}
	}
	raw, found := root[key]
	if found {
		if nested, ok := raw.(map[string]any); ok {
			return nested
		}
	}
	nested := map[string]any{}
	root[key] = nested
	return nested
}

func (r *EKSKarpenterBootstrapperReconciler) upsertManagedUnstructured(ctx context.Context, owner *clusterv1.Cluster, obj *unstructured.Unstructured, mutate func(*unstructured.Unstructured) error) (bool, error) {
	if owner == nil {
		return false, fmt.Errorf("owner cluster is nil")
	}
	if obj == nil {
		return false, fmt.Errorf("object is nil")
	}
	key := client.ObjectKey{Namespace: obj.GetNamespace(), Name: obj.GetName()}

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(obj.GroupVersionKind())
	if err := r.Get(ctx, key, existing); err != nil {
		if !apierrors.IsNotFound(err) {
			return false, err
		}
		if err := mutate(obj); err != nil {
			return false, err
		}
		if err := popMutationError(obj); err != nil {
			return false, err
		}
		if err := controllerutil.SetOwnerReference(owner, obj, r.Scheme); err != nil {
			return false, err
		}
		if err := r.Create(ctx, obj); err != nil {
			return false, err
		}
		return true, nil
	}

	if !isManagedByBootstrapper(existing.GetAnnotations()) {
		if !coreeks.IsUnmanagedTakeoverEnabled(owner.GetAnnotations()) {
			return false, nil
		}
		before := existing.DeepCopy()
		if err := mutate(existing); err != nil {
			return false, err
		}
		if err := popMutationError(existing); err != nil {
			return false, err
		}
		if err := controllerutil.SetOwnerReference(owner, existing, r.Scheme); err != nil {
			return false, err
		}
		if !equality.Semantic.DeepEqual(before, existing) {
			if err := r.Update(ctx, existing); err != nil {
				return false, err
			}
		}
		msg := fmt.Sprintf(
			"took over unmanaged %s %s/%s because %q is enabled",
			existing.GroupVersionKind().Kind,
			existing.GetNamespace(),
			existing.GetName(),
			coreeks.AllowUnmanagedTakeoverAnnotationKey,
		)
		r.emitEvent(owner, corev1.EventTypeNormal, reasonResourceTakeover, msg)
		return true, nil
	}

	before := existing.DeepCopy()
	if err := mutate(existing); err != nil {
		return false, err
	}
	if err := popMutationError(existing); err != nil {
		return false, err
	}
	if err := controllerutil.SetOwnerReference(owner, existing, r.Scheme); err != nil {
		return false, err
	}
	if equality.Semantic.DeepEqual(before, existing) {
		return true, nil
	}
	if err := r.Update(ctx, existing); err != nil {
		return false, err
	}
	return true, nil
}

func isManagedByBootstrapper(annotations map[string]string) bool {
	if len(annotations) == 0 {
		return false
	}
	return strings.TrimSpace(annotations[coreeks.ManagedByAnnotationKey]) == karpenterManagedByValue
}

func setManagedBy(u *unstructured.Unstructured) {
	if u == nil {
		return
	}
	ann := u.GetAnnotations()
	if ann == nil {
		ann = map[string]string{}
	}
	ann[coreeks.ManagedByAnnotationKey] = karpenterManagedByValue
	u.SetAnnotations(ann)
}

func setRegionAnnotation(u *unstructured.Unstructured, region string) {
	if u == nil {
		return
	}
	ann := u.GetAnnotations()
	if ann == nil {
		ann = map[string]string{}
	}
	ann[coreeks.ACKRegionMetadataAnnotationKey] = region
	u.SetAnnotations(ann)
}

func setClusterLabel(u *unstructured.Unstructured, clusterName string) {
	if u == nil {
		return
	}
	l := u.GetLabels()
	if l == nil {
		l = map[string]string{}
	}
	l[capiClusterNameLabelKey] = clusterName
	u.SetLabels(l)
}

func newUnstructured(gvk schema.GroupVersionKind, namespace, name string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)
	u.SetNamespace(namespace)
	u.SetName(name)
	// NOTE: do not overwrite u.Object after SetName/SetNamespace.
	// Those helpers populate metadata fields inside u.Object.
	return u
}

func mustSetNestedString(u *unstructured.Unstructured, value string, fields ...string) {
	mustSetNestedField(u, value, fields...)
}

func mustSetNestedSlice(u *unstructured.Unstructured, value []any, fields ...string) {
	mustSetNestedField(u, value, fields...)
}

func mustSetNestedField(u *unstructured.Unstructured, value any, fields ...string) {
	if u.Object == nil {
		u.Object = map[string]any{}
	}
	if err := unstructured.SetNestedField(u.Object, value, fields...); err != nil {
		setMutationError(u, err)
	}
}

func setMutationError(u *unstructured.Unstructured, err error) {
	if u == nil || err == nil {
		return
	}
	if u.Object == nil {
		u.Object = map[string]any{}
	}
	// Preserve the first error to keep the root cause when multiple nested writes fail.
	if _, exists := u.Object[mutationErrorObjectKey]; exists {
		return
	}
	u.Object[mutationErrorObjectKey] = err.Error()
}

func popMutationError(u *unstructured.Unstructured) error {
	if u == nil || u.Object == nil {
		return nil
	}
	raw, found := u.Object[mutationErrorObjectKey]
	if !found {
		return nil
	}
	delete(u.Object, mutationErrorObjectKey)
	msg, _ := raw.(string)
	msg = strings.TrimSpace(msg)
	if msg == "" {
		msg = "unknown nested field mutation error"
	}
	return fmt.Errorf("mutate object fields: %s", msg)
}

func isNoMatchError(err error) bool {
	if err == nil {
		return false
	}
	// Server-side error message for missing CRD is not strongly typed.
	msg := err.Error()
	return strings.Contains(msg, "no matches for kind") || strings.Contains(msg, "could not find the requested resource")
}

func (r *EKSKarpenterBootstrapperReconciler) getACKCluster(ctx context.Context, namespace, name string) (*unstructured.Unstructured, error) {
	ack := &unstructured.Unstructured{}
	ack.SetGroupVersionKind(ackClusterGVK)
	if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, ack); err != nil {
		return nil, err
	}
	return ack, nil
}

func (r *EKSKarpenterBootstrapperReconciler) emitEvent(cluster *clusterv1.Cluster, eventType, reason, message string) {
	if r == nil || r.Recorder == nil || cluster == nil {
		return
	}
	if !controllerEventState.shouldEmit("eks-karpenter-bootstrapper", cluster.Namespace, cluster.Name, eventType, reason, message) {
		return
	}
	r.Recorder.Event(cluster, eventType, reason, message)
}

func (r *EKSKarpenterBootstrapperReconciler) failureBackoff() time.Duration {
	if r.FailureBackoff == 0 {
		return 30 * time.Second
	}
	return r.FailureBackoff
}

func (r *EKSKarpenterBootstrapperReconciler) steadyStateRequeue() time.Duration {
	if r.SteadyStateRequeue == 0 {
		return 10 * time.Minute
	}
	return r.SteadyStateRequeue
}

func (r *EKSKarpenterBootstrapperReconciler) now() time.Time {
	if r.Now == nil {
		return time.Now().UTC()
	}
	return r.Now().UTC()
}

func (r *EKSKarpenterBootstrapperReconciler) validateFargateSubnets(ctx context.Context, region string, nodeSubnetIDs []string) (fargateSubnetValidationResult, error) {
	if r != nil && r.ValidateSubnets != nil {
		return r.ValidateSubnets(ctx, region, nodeSubnetIDs)
	}
	return validateFargateSubnets(ctx, region, nodeSubnetIDs)
}

func (r *EKSKarpenterBootstrapperReconciler) resolveOIDCThumbprints(ctx context.Context, cluster *clusterv1.Cluster, issuerURL string) ([]string, error) {
	if cluster == nil || cluster.Annotations == nil {
		return nil, nil
	}
	if !strings.EqualFold(strings.TrimSpace(cluster.Annotations[oidcThumbprintAutoAnnotation]), "enabled") &&
		!strings.EqualFold(strings.TrimSpace(cluster.Annotations[oidcThumbprintAutoAnnotation]), "true") {
		return nil, nil
	}

	if r != nil && r.ResolveOIDCThumbprints != nil {
		return r.ResolveOIDCThumbprints(ctx, issuerURL)
	}
	return defaultResolveOIDCThumbprints(ctx, issuerURL)
}

func defaultResolveOIDCThumbprints(ctx context.Context, issuerURL string) ([]string, error) {
	issuerURL = strings.TrimSpace(issuerURL)
	if issuerURL == "" {
		return nil, fmt.Errorf("issuer URL is empty")
	}
	parsed, err := url.Parse(issuerURL)
	if err != nil {
		return nil, fmt.Errorf("parse issuer URL: %w", err)
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return nil, fmt.Errorf("issuer URL host is empty")
	}
	port := strings.TrimSpace(parsed.Port())
	if port == "" {
		port = "443"
	}

	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(host, port), &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ServerName:         host,
		InsecureSkipVerify: true,
	})
	if err != nil {
		return nil, fmt.Errorf("tls dial issuer: %w", err)
	}
	defer func() { _ = conn.Close() }()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	peerCerts := conn.ConnectionState().PeerCertificates
	if len(peerCerts) == 0 {
		return nil, fmt.Errorf("issuer certificate chain is empty")
	}

	roots, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("%w: load system cert pool: %v", errOIDCThumbprintUnverified, err)
	}
	if roots == nil {
		roots = x509.NewCertPool()
	}
	intermediates := x509.NewCertPool()
	for _, cert := range peerCerts[1:] {
		intermediates.AddCert(cert)
	}
	chains, err := peerCerts[0].Verify(x509.VerifyOptions{
		DNSName:       host,
		Roots:         roots,
		Intermediates: intermediates,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: verify issuer certificate chain: %v", errOIDCThumbprintUnverified, err)
	}
	verifiedChain := selectLongestVerifiedChain(chains)
	if len(verifiedChain) == 0 {
		return nil, fmt.Errorf("%w: verified chains are empty", errOIDCThumbprintUnverified)
	}
	topIntermediate, err := selectTopIntermediateCACert(verifiedChain)
	if err != nil {
		return nil, err
	}
	sum := sha1.Sum(topIntermediate.Raw)
	return []string{strings.ToLower(hex.EncodeToString(sum[:]))}, nil
}

func selectLongestVerifiedChain(chains [][]*x509.Certificate) []*x509.Certificate {
	if len(chains) == 0 {
		return nil
	}
	longest := chains[0]
	for _, chain := range chains[1:] {
		if len(chain) > len(longest) {
			longest = chain
		}
	}
	return longest
}

func selectTopIntermediateCACert(chain []*x509.Certificate) (*x509.Certificate, error) {
	if len(chain) == 0 {
		return nil, fmt.Errorf("%w: verified chain is empty", errOIDCThumbprintUnverified)
	}
	for i := len(chain) - 1; i >= 0; i-- {
		cert := chain[i]
		if cert == nil || !cert.IsCA {
			continue
		}
		// Skip self-signed root; IAM expects top intermediate CA thumbprint.
		if isSelfSignedCertificate(cert) {
			continue
		}
		if len(cert.Raw) == 0 {
			return nil, fmt.Errorf("%w: selected CA certificate has empty raw bytes", errOIDCThumbprintUnverified)
		}
		return cert, nil
	}
	return nil, fmt.Errorf("%w: no top intermediate CA certificate in verified chain", errOIDCThumbprintUnverified)
}

func isSelfSignedCertificate(cert *x509.Certificate) bool {
	if cert == nil {
		return false
	}
	return cert.Subject.String() == cert.Issuer.String()
}

func (r *EKSKarpenterBootstrapperReconciler) isAPIAvailable(gvk schema.GroupVersionKind) bool {
	if r == nil || r.RESTMapper == nil {
		return true
	}
	_, err := r.RESTMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	return err == nil
}

func (r *EKSKarpenterBootstrapperReconciler) areAPIsAvailable(gvks ...schema.GroupVersionKind) bool {
	for _, gvk := range gvks {
		if !r.isAPIAvailable(gvk) {
			return false
		}
	}
	return true
}

func (r *EKSKarpenterBootstrapperReconciler) missingAPIs(gvks ...schema.GroupVersionKind) []schema.GroupVersionKind {
	out := []schema.GroupVersionKind{}
	for _, gvk := range gvks {
		if r.isAPIAvailable(gvk) {
			continue
		}
		out = append(out, gvk)
	}
	return out
}

func joinGVKs(gvks []schema.GroupVersionKind) string {
	if len(gvks) == 0 {
		return ""
	}
	values := make([]string, 0, len(gvks))
	for _, gvk := range gvks {
		values = append(values, gvk.String())
	}
	sort.Strings(values)
	return strings.Join(values, ", ")
}

func normalizeIssuerHostPath(issuerURL string) (string, error) {
	trimmed := strings.TrimSpace(issuerURL)
	if trimmed == "" {
		return "", fmt.Errorf("issuer URL is empty")
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("parse issuer url %q: %w", issuerURL, err)
	}
	if u.Scheme != "https" {
		return "", fmt.Errorf("issuer url scheme must be https: %q", issuerURL)
	}
	if u.Host == "" {
		return "", fmt.Errorf("issuer url host is empty: %q", issuerURL)
	}
	// IAM OIDC provider path uses host + path (no leading https://).
	return strings.TrimSuffix(u.Host+u.Path, "/"), nil
}

func shortenAWSName(name string, max int) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	if max <= 0 {
		return trimmed
	}
	if len(trimmed) <= max {
		return trimmed
	}
	sum := sha256.Sum256([]byte(trimmed))
	suffix := hex.EncodeToString(sum[:])[:12]
	keep := max - 13
	if keep < 1 {
		return suffix[:max]
	}
	return trimmed[:keep] + "-" + suffix
}

func readTopologyStringSlice(cluster *clusterv1.Cluster, variableName string) ([]string, bool, error) {
	if cluster == nil || !cluster.Spec.Topology.IsDefined() {
		return nil, false, nil
	}
	for i := range cluster.Spec.Topology.Variables {
		v := cluster.Spec.Topology.Variables[i]
		if v.Name != variableName {
			continue
		}
		var out []string
		if err := json.Unmarshal(v.Value.Raw, &out); err != nil {
			return nil, false, fmt.Errorf("unmarshal topology variable %q: %w", variableName, err)
		}
		return out, true, nil
	}
	return nil, false, nil
}

func (r *EKSKarpenterBootstrapperReconciler) ensureTopologyStringSliceVariable(ctx context.Context, cluster *clusterv1.Cluster, variableName string, desired []string) (bool, error) {
	if r == nil || cluster == nil {
		return false, nil
	}
	if !cluster.Spec.Topology.IsDefined() {
		return false, nil
	}

	// Normalize desired values (trim + drop empties, keep order).
	norm := []string{}
	for _, v := range desired {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		norm = append(norm, v)
	}

	before := cluster.DeepCopy()
	changed, err := setTopologyStringSliceVariable(cluster, variableName, norm)
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}
	if err := r.Patch(ctx, cluster, client.MergeFrom(before)); err != nil {
		return false, err
	}
	return true, nil
}

func setTopologyStringSliceVariable(cluster *clusterv1.Cluster, variableName string, desired []string) (bool, error) {
	if cluster == nil || !cluster.Spec.Topology.IsDefined() {
		return false, nil
	}
	if variableName = strings.TrimSpace(variableName); variableName == "" {
		return false, fmt.Errorf("variable name is empty")
	}

	raw, err := json.Marshal(desired)
	if err != nil {
		return false, fmt.Errorf("marshal desired value for topology variable %q: %w", variableName, err)
	}

	for i := range cluster.Spec.Topology.Variables {
		v := &cluster.Spec.Topology.Variables[i]
		if v.Name != variableName {
			continue
		}
		var current []string
		if len(v.Value.Raw) > 0 {
			if err := json.Unmarshal(v.Value.Raw, &current); err != nil {
				return false, fmt.Errorf("unmarshal current topology variable %q: %w", variableName, err)
			}
		}
		if equality.Semantic.DeepEqual(current, desired) {
			return false, nil
		}
		v.Value.Raw = raw
		return true, nil
	}

	// Variable does not exist; append it.
	newVar := clusterv1.ClusterVariable{Name: variableName}
	newVar.Value.Raw = raw
	cluster.Spec.Topology.Variables = append(cluster.Spec.Topology.Variables, newVar)
	return true, nil
}

func buildKarpenterControllerAssumeRolePolicyDocument(oidcProviderARN, issuerHostPath string) string {
	// issuerHostPath looks like: oidc.eks.<region>.amazonaws.com/id/XXXXXXXX
	issuer := strings.TrimSpace(issuerHostPath)
	return fmt.Sprintf(
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Federated":%q},"Action":"sts:AssumeRoleWithWebIdentity","Condition":{"StringEquals":{"%s:aud":"sts.amazonaws.com","%s:sub":"system:serviceaccount:karpenter:karpenter"}}}]}`,
		oidcProviderARN,
		issuer,
		issuer,
	)
}

func buildKarpenterControllerPolicyDocument(region, accountID, eksClusterName, nodeRoleARN string) string {
	// Baseline: terraform-aws-modules/terraform-aws-eks (modules/karpenter) v1 policy.
	// For a dev sample we keep this embedded to avoid external dependencies.
	cluster := strings.TrimSpace(eksClusterName)
	region = strings.TrimSpace(region)
	accountID = strings.TrimSpace(accountID)
	nodeRoleARN = strings.TrimSpace(nodeRoleARN)

	return fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowScopedEC2InstanceAccessActions",
      "Effect": "Allow",
      "Resource": [
        "arn:aws:ec2:%[1]s::image/*",
        "arn:aws:ec2:%[1]s::snapshot/*",
        "arn:aws:ec2:%[1]s:*:security-group/*",
        "arn:aws:ec2:%[1]s:*:subnet/*"
      ],
      "Action": [
        "ec2:RunInstances",
        "ec2:CreateFleet"
      ]
    },
    {
      "Sid": "AllowScopedEC2LaunchTemplateAccessActions",
      "Effect": "Allow",
      "Resource": ["arn:aws:ec2:%[1]s:*:launch-template/*"],
      "Action": [
        "ec2:RunInstances",
        "ec2:CreateFleet"
      ],
      "Condition": {
        "StringEquals": {
          "aws:ResourceTag/kubernetes.io/cluster/%[2]s": "owned"
        },
        "StringLike": {
          "aws:ResourceTag/karpenter.sh/nodepool": "*"
        }
      }
    },
    {
      "Sid": "AllowScopedEC2InstanceActionsWithTags",
      "Effect": "Allow",
      "Resource": [
        "arn:aws:ec2:%[1]s:*:fleet/*",
        "arn:aws:ec2:%[1]s:*:instance/*",
        "arn:aws:ec2:%[1]s:*:volume/*",
        "arn:aws:ec2:%[1]s:*:network-interface/*",
        "arn:aws:ec2:%[1]s:*:launch-template/*",
        "arn:aws:ec2:%[1]s:*:spot-instances-request/*"
      ],
      "Action": [
        "ec2:RunInstances",
        "ec2:CreateFleet",
        "ec2:CreateLaunchTemplate"
      ],
      "Condition": {
        "StringEquals": {
          "aws:RequestTag/kubernetes.io/cluster/%[2]s": "owned",
          "aws:RequestTag/eks:eks-cluster-name": "%[2]s"
        },
        "StringLike": {
          "aws:RequestTag/karpenter.sh/nodepool": "*"
        }
      }
    },
    {
      "Sid": "AllowScopedResourceCreationTagging",
      "Effect": "Allow",
      "Resource": [
        "arn:aws:ec2:%[1]s:*:fleet/*",
        "arn:aws:ec2:%[1]s:*:instance/*",
        "arn:aws:ec2:%[1]s:*:volume/*",
        "arn:aws:ec2:%[1]s:*:network-interface/*",
        "arn:aws:ec2:%[1]s:*:launch-template/*",
        "arn:aws:ec2:%[1]s:*:spot-instances-request/*"
      ],
      "Action": ["ec2:CreateTags"],
      "Condition": {
        "StringEquals": {
          "aws:RequestTag/kubernetes.io/cluster/%[2]s": "owned",
          "aws:RequestTag/eks:eks-cluster-name": "%[2]s",
          "ec2:CreateAction": [
            "RunInstances",
            "CreateFleet",
            "CreateLaunchTemplate"
          ]
        },
        "StringLike": {
          "aws:RequestTag/karpenter.sh/nodepool": "*"
        }
      }
    },
    {
      "Sid": "AllowScopedResourceTagging",
      "Effect": "Allow",
      "Resource": ["arn:aws:ec2:%[1]s:*:instance/*"],
      "Action": ["ec2:CreateTags"],
      "Condition": {
        "StringEquals": {
          "aws:ResourceTag/kubernetes.io/cluster/%[2]s": "owned"
        },
        "StringLike": {
          "aws:ResourceTag/karpenter.sh/nodepool": "*"
        },
        "StringEqualsIfExists": {
          "aws:RequestTag/eks:eks-cluster-name": "%[2]s"
        },
        "ForAllValues:StringEquals": {
          "aws:TagKeys": [
            "eks:eks-cluster-name",
            "karpenter.sh/nodeclaim",
            "Name"
          ]
        }
      }
    },
    {
      "Sid": "AllowScopedDeletion",
      "Effect": "Allow",
      "Resource": [
        "arn:aws:ec2:%[1]s:*:instance/*",
        "arn:aws:ec2:%[1]s:*:launch-template/*"
      ],
      "Action": [
        "ec2:TerminateInstances",
        "ec2:DeleteLaunchTemplate"
      ],
      "Condition": {
        "StringEquals": {
          "aws:ResourceTag/kubernetes.io/cluster/%[2]s": "owned"
        },
        "StringLike": {
          "aws:ResourceTag/karpenter.sh/nodepool": "*"
        }
      }
    },
    {
      "Sid": "AllowRegionalReadActions",
      "Effect": "Allow",
      "Resource": "*",
      "Action": [
        "ec2:DescribeAvailabilityZones",
        "ec2:DescribeImages",
        "ec2:DescribeInstances",
        "ec2:DescribeInstanceTypeOfferings",
        "ec2:DescribeInstanceTypes",
        "ec2:DescribeLaunchTemplates",
        "ec2:DescribeSecurityGroups",
        "ec2:DescribeSpotPriceHistory",
        "ec2:DescribeSubnets",
        "ec2:DescribeLaunchTemplateVersions",
        "ec2:DescribeVpcs"
      ],
      "Condition": {
        "StringEquals": {
          "aws:RequestedRegion": "%[1]s"
        }
      }
    },
    {
      "Sid": "AllowSSMReadActions",
      "Effect": "Allow",
      "Resource": ["arn:aws:ssm:%[1]s::parameter/aws/service/*"],
      "Action": ["ssm:GetParameter"]
    },
    {
      "Sid": "AllowPricingReadActions",
      "Effect": "Allow",
      "Resource": "*",
      "Action": ["pricing:GetProducts"]
    },
    {
      "Sid": "AllowPassingInstanceRole",
      "Effect": "Allow",
      "Resource": [%[3]q],
      "Action": ["iam:PassRole"],
      "Condition": {
        "StringEquals": {
          "iam:PassedToService": "ec2.amazonaws.com"
        }
      }
    },
    {
      "Sid": "AllowScopedInstanceProfileCreationActions",
      "Effect": "Allow",
      "Resource": ["arn:aws:iam::%[4]s:instance-profile/*"],
      "Action": ["iam:CreateInstanceProfile"],
      "Condition": {
        "StringEquals": {
          "aws:RequestTag/kubernetes.io/cluster/%[2]s": "owned",
          "aws:RequestTag/eks:eks-cluster-name": "%[2]s",
          "aws:RequestTag/topology.kubernetes.io/region": "%[1]s"
        },
        "StringLike": {
          "aws:RequestTag/karpenter.k8s.aws/ec2nodeclass": "*"
        }
      }
    },
    {
      "Sid": "AllowScopedInstanceProfileTagActions",
      "Effect": "Allow",
      "Resource": ["arn:aws:iam::%[4]s:instance-profile/*"],
      "Action": ["iam:TagInstanceProfile"],
      "Condition": {
        "StringEquals": {
          "aws:ResourceTag/kubernetes.io/cluster/%[2]s": "owned",
          "aws:RequestTag/kubernetes.io/cluster/%[2]s": "owned",
          "aws:RequestTag/eks:eks-cluster-name": "%[2]s",
          "aws:ResourceTag/topology.kubernetes.io/region": "%[1]s",
          "aws:RequestTag/topology.kubernetes.io/region": "%[1]s"
        },
        "StringLike": {
          "aws:ResourceTag/karpenter.k8s.aws/ec2nodeclass": "*",
          "aws:RequestTag/karpenter.k8s.aws/ec2nodeclass": "*"
        }
      }
    },
    {
      "Sid": "AllowScopedInstanceProfileActions",
      "Effect": "Allow",
      "Resource": ["arn:aws:iam::%[4]s:instance-profile/*"],
      "Action": [
        "iam:AddRoleToInstanceProfile",
        "iam:RemoveRoleFromInstanceProfile",
        "iam:DeleteInstanceProfile"
      ],
      "Condition": {
        "StringEquals": {
          "aws:ResourceTag/kubernetes.io/cluster/%[2]s": "owned",
          "aws:ResourceTag/topology.kubernetes.io/region": "%[1]s"
        },
        "StringLike": {
          "aws:ResourceTag/karpenter.k8s.aws/ec2nodeclass": "*"
        }
      }
    },
    {
      "Sid": "AllowInstanceProfileReadActions",
      "Effect": "Allow",
      "Resource": ["arn:aws:iam::%[4]s:instance-profile/*"],
      "Action": ["iam:GetInstanceProfile"]
    },
    {
      "Sid": "AllowAPIServerEndpointDiscovery",
      "Effect": "Allow",
      "Resource": ["arn:aws:eks:%[1]s:%[4]s:cluster/%[2]s"],
      "Action": ["eks:DescribeCluster"]
    }
  ]
}`,
		region,
		cluster,
		nodeRoleARN,
		accountID,
	)
}
