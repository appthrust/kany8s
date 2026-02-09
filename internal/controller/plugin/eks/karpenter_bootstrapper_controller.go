package eks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	karpenterEnableLabelKey   = "eks.kany8s.io/karpenter"
	karpenterEnableLabelValue = "enabled"

	karpenterManagedByValue = "eks-karpenter-bootstrapper"

	capiClusterNameLabelKey = "cluster.x-k8s.io/cluster-name"

	// TODO: make configurable.
	defaultKarpenterChartVersion = "1.0.8"
)

var (
	ackAccessEntryGVK        = schema.GroupVersionKind{Group: "eks.services.k8s.aws", Version: "v1alpha1", Kind: "AccessEntry"}
	ackFargateProfileGVK     = schema.GroupVersionKind{Group: "eks.services.k8s.aws", Version: "v1alpha1", Kind: "FargateProfile"}
	ackIAMPolicyGVK          = schema.GroupVersionKind{Group: "iam.services.k8s.aws", Version: "v1alpha1", Kind: "Policy"}
	ackIAMRoleGVK            = schema.GroupVersionKind{Group: "iam.services.k8s.aws", Version: "v1alpha1", Kind: "Role"}
	ackIAMInstanceProfileGVK = schema.GroupVersionKind{Group: "iam.services.k8s.aws", Version: "v1alpha1", Kind: "InstanceProfile"}
	ackOIDCProviderGVK       = schema.GroupVersionKind{Group: "iam.services.k8s.aws", Version: "v1alpha1", Kind: "OpenIDConnectProvider"}
	ackSecurityGroupGVK      = schema.GroupVersionKind{Group: "ec2.services.k8s.aws", Version: "v1alpha1", Kind: "SecurityGroup"}

	fluxOCIRepositoryGVK = schema.GroupVersionKind{Group: "source.toolkit.fluxcd.io", Version: "v1", Kind: "OCIRepository"}
	fluxHelmReleaseGVK   = schema.GroupVersionKind{Group: "helm.toolkit.fluxcd.io", Version: "v2", Kind: "HelmRelease"}

	clusterResourceSetGVK = schema.GroupVersionKind{Group: "addons.cluster.x-k8s.io", Version: "v1beta2", Kind: "ClusterResourceSet"}
)

const (
	reasonKarpenterDisabled         = "KarpenterDisabled"
	reasonTopologyMissing           = "TopologyMissing"
	reasonTopologyVariableMissing   = "TopologyVariableMissing"
	reasonTopologyVariablePatched   = "TopologyVariablePatched"
	reasonACKClusterNotFoundKarp    = "ACKClusterNotFound"
	reasonACKClusterNotReadyKarp    = "ACKClusterNotReady"
	reasonFluxNotInstalled          = "FluxNotInstalled"
	reasonResourceOwnership         = "ResourceOwnershipConflict"
	reasonBootstrapperReconciled    = "BootstrapperReconciled"
	reasonWorkloadRolloutRestarted  = "WorkloadRolloutRestarted"
	reasonAWSPrerequisitesNotReady  = "AWSPrerequisitesNotReady"
	reasonNodeSecurityGroupNotReady = "NodeSecurityGroupNotReady"
)

type EKSKarpenterBootstrapperReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	Recorder       recordEventEmitter
	TokenGenerator coreeks.TokenGenerator
	Now            func() time.Time

	FailureBackoff     time.Duration
	SteadyStateRequeue time.Duration
}

// nolint:gocyclo
func (r *EKSKarpenterBootstrapperReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	cluster := &clusterv1.Cluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if cluster.DeletionTimestamp != nil {
		// Best-effort cleanup on CAPI Cluster deletion.
		// Note: Cluster API already sets a finalizer on Cluster, so we should have enough time to
		// terminate Karpenter-provisioned EC2 instances without adding an extra finalizer.
		return r.reconcileDeletingCluster(ctx, cluster)
	}

	if !isKarpenterEnabled(cluster) {
		return ctrl.Result{}, nil
	}
	if err := r.ensureClusterNameLabel(ctx, cluster); err != nil {
		return ctrl.Result{}, err
	}

	capiClusterName, eksClusterName, ackClusterName := resolveClusterNames(cluster)
	if !cluster.Spec.Topology.IsDefined() {
		msg := "Cluster.spec.topology is required for BYO variables (subnet IDs / security group IDs)"
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonTopologyMissing, msg)
		return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
	}

	subnetIDs, ok, err := readTopologyStringSlice(cluster, "vpc-subnet-ids")
	if err != nil {
		return ctrl.Result{}, err
	}
	if !ok || len(subnetIDs) < 2 {
		msg := "missing topology variable vpc-subnet-ids (need at least 2 subnets)"
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonTopologyVariableMissing, msg)
		return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
	}

	securityGroupIDs, ok, err := readTopologyStringSlice(cluster, "vpc-security-group-ids")
	if err != nil {
		return ctrl.Result{}, err
	}
	if !ok {
		securityGroupIDs = nil
	}

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
		msg := fmt.Sprintf("waiting for ACK EKS Cluster spec.accessConfig.authenticationMode to be set (need %q for AccessEntry-based node join) on %s/%s", "API_AND_CONFIG_MAP", cluster.Namespace, ackClusterName)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonAWSPrerequisitesNotReady, msg)
		return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
	}
	if authMode != "API_AND_CONFIG_MAP" {
		msg := fmt.Sprintf("EKS accessConfig.authenticationMode must be %q for AccessEntry-based node join (got %q) on %s/%s", "API_AND_CONFIG_MAP", authMode, cluster.Namespace, ackClusterName)
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

	// Default reconcile cadence, overridden when prerequisites are not ready.
	requeueAfter := r.steadyStateRequeue()

	// Ensure we have at least one security group for Karpenter nodes.
	// If the topology variable vpc-security-group-ids is empty, we create a default node SecurityGroup via ACK,
	// then inject the created security group ID back into Cluster.spec.topology.variables so that the
	// ClusterClass patches can propagate it to the control plane (EKS) and future reconciles converge.
	if len(securityGroupIDs) == 0 {
		id, managed, err := r.ensureNodeSecurityGroup(ctx, cluster, region, eksClusterName, subnetIDs)
		if err != nil {
			msg := fmt.Sprintf("failed to reconcile node SecurityGroup: %v", err)
			r.emitEvent(cluster, corev1.EventTypeWarning, reasonAWSPrerequisitesNotReady, msg)
			return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
		}
		if !managed {
			msg := fmt.Sprintf("node SecurityGroup exists and is not managed by %s", karpenterManagedByValue)
			r.emitEvent(cluster, corev1.EventTypeWarning, reasonResourceOwnership, msg)
			return ctrl.Result{RequeueAfter: r.steadyStateRequeue()}, nil
		}
		if id == "" {
			msg := "waiting for node SecurityGroup to be created (status.id is empty)"
			r.emitEvent(cluster, corev1.EventTypeNormal, reasonNodeSecurityGroupNotReady, msg)
			requeueAfter = r.failureBackoff()
		} else {
			patched, err := r.ensureTopologyStringSliceVariable(ctx, cluster, "vpc-security-group-ids", []string{id})
			if err != nil {
				return ctrl.Result{}, err
			}
			if patched {
				msg := fmt.Sprintf("patched Cluster.spec.topology.variables[%q] with %q", "vpc-security-group-ids", id)
				r.emitEvent(cluster, corev1.EventTypeNormal, reasonTopologyVariablePatched, msg)
				// Let CAPI Topology / ControlPlane reconcile propagate the injected SG to ACK EKS Cluster.
				return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
			}
			securityGroupIDs = []string{id}
		}
	}

	issuerHostPath, err := normalizeIssuerHostPath(issuerURL)
	if err != nil {
		return ctrl.Result{}, err
	}
	oidcProviderARN := fmt.Sprintf("arn:aws:iam::%s:oidc-provider/%s", accountID, issuerHostPath)

	// 1) Ensure OIDC provider for IRSA.
	oidcName := fmt.Sprintf("%s-oidc-provider", capiClusterName)
	if ok, err := r.ensureOIDCProvider(ctx, cluster, oidcName, region, issuerURL); err != nil {
		return ctrl.Result{}, err
	} else if !ok {
		msg := fmt.Sprintf("OIDC provider %s/%s exists and is not managed by %s", cluster.Namespace, oidcName, karpenterManagedByValue)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonResourceOwnership, msg)
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
	fargateRoleName := fmt.Sprintf("%s-fargate-pod-execution", capiClusterName)
	fargateRoleAWSName := shortenAWSName(fmt.Sprintf("%s-fargate-pod-execution", eksClusterName), 64)

	controllerRoleARN := fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, controllerRoleAWSName)
	nodeRoleARN := fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, nodeRoleAWSName)

	policyDoc := buildKarpenterControllerPolicyDocument(region, accountID, eksClusterName, nodeRoleARN)
	if ok, err := r.ensureIAMPolicy(ctx, cluster, controllerPolicyName, region, policyDoc, shortenAWSName(fmt.Sprintf("%s-karpenter-controller", eksClusterName), 128)); err != nil {
		return ctrl.Result{}, err
	} else if !ok {
		msg := fmt.Sprintf("IAM Policy %s/%s exists and is not managed by %s", cluster.Namespace, controllerPolicyName, karpenterManagedByValue)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonResourceOwnership, msg)
		return ctrl.Result{RequeueAfter: r.steadyStateRequeue()}, nil
	}

	assumePolicy := buildKarpenterControllerAssumeRolePolicyDocument(oidcProviderARN, issuerHostPath)
	if ok, err := r.ensureIAMRoleForIRSA(ctx, cluster, controllerRoleName, region, controllerRoleAWSName, assumePolicy, []string{controllerPolicyName}); err != nil {
		return ctrl.Result{}, err
	} else if !ok {
		msg := fmt.Sprintf("IAM Role %s/%s exists and is not managed by %s", cluster.Namespace, controllerRoleName, karpenterManagedByValue)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonResourceOwnership, msg)
		return ctrl.Result{RequeueAfter: r.steadyStateRequeue()}, nil
	}

	nodeManagedPolicies := []string{
		"arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy",
		"arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy",
		"arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly",
	}
	if ok, err := r.ensureIAMRoleForEC2(ctx, cluster, nodeRoleName, region, nodeRoleAWSName, nodeManagedPolicies); err != nil {
		return ctrl.Result{}, err
	} else if !ok {
		msg := fmt.Sprintf("IAM Role %s/%s exists and is not managed by %s", cluster.Namespace, nodeRoleName, karpenterManagedByValue)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonResourceOwnership, msg)
		return ctrl.Result{RequeueAfter: r.steadyStateRequeue()}, nil
	}
	if ok, err := r.ensureIAMInstanceProfile(ctx, cluster, nodeInstanceProfileName, region, nodeInstanceProfileAWSName, nodeRoleName); err != nil {
		return ctrl.Result{}, err
	} else if !ok {
		msg := fmt.Sprintf("IAM InstanceProfile %s/%s exists and is not managed by %s", cluster.Namespace, nodeInstanceProfileName, karpenterManagedByValue)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonResourceOwnership, msg)
		return ctrl.Result{RequeueAfter: r.steadyStateRequeue()}, nil
	}

	fargateManagedPolicies := []string{
		"arn:aws:iam::aws:policy/AmazonEKSFargatePodExecutionRolePolicy",
	}
	if ok, err := r.ensureIAMRoleForFargatePods(ctx, cluster, fargateRoleName, region, fargateRoleAWSName, fargateManagedPolicies); err != nil {
		return ctrl.Result{}, err
	} else if !ok {
		msg := fmt.Sprintf("IAM Role %s/%s exists and is not managed by %s", cluster.Namespace, fargateRoleName, karpenterManagedByValue)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonResourceOwnership, msg)
		return ctrl.Result{RequeueAfter: r.steadyStateRequeue()}, nil
	}

	// 3) Ensure EKS AccessEntry (node join without aws-auth).
	accessEntryName := fmt.Sprintf("%s-karpenter-node", capiClusterName)
	if ok, err := r.ensureAccessEntry(ctx, cluster, accessEntryName, region, ackClusterName, nodeRoleARN); err != nil {
		return ctrl.Result{}, err
	} else if !ok {
		msg := fmt.Sprintf("AccessEntry %s/%s exists and is not managed by %s", cluster.Namespace, accessEntryName, karpenterManagedByValue)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonResourceOwnership, msg)
		return ctrl.Result{RequeueAfter: r.steadyStateRequeue()}, nil
	}

	// 4) Ensure Fargate profiles for bootstrap compute.
	karpenterFargateObjName := fmt.Sprintf("%s-fargate-karpenter", capiClusterName)
	corednsFargateObjName := fmt.Sprintf("%s-fargate-coredns", capiClusterName)
	if ok, err := r.ensureFargateProfile(ctx, cluster, karpenterFargateObjName, region, ackClusterName, "karpenter", fargateRoleName, subnetIDs, []map[string]any{{"namespace": "karpenter"}}); err != nil {
		msg := fmt.Sprintf(
			"failed to reconcile FargateProfile %s/%s: %v (ensure vpc-subnet-ids are private subnets and satisfy EKS Fargate requirements)",
			cluster.Namespace,
			karpenterFargateObjName,
			err,
		)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonAWSPrerequisitesNotReady, msg)
		return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
	} else if !ok {
		msg := fmt.Sprintf("FargateProfile %s/%s exists and is not managed by %s", cluster.Namespace, karpenterFargateObjName, karpenterManagedByValue)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonResourceOwnership, msg)
		return ctrl.Result{RequeueAfter: r.steadyStateRequeue()}, nil
	}
	if ok, err := r.ensureFargateProfile(ctx, cluster, corednsFargateObjName, region, ackClusterName, "coredns", fargateRoleName, subnetIDs, []map[string]any{{
		"namespace": "kube-system",
		"labels":    map[string]any{"k8s-app": "kube-dns"},
	}}); err != nil {
		msg := fmt.Sprintf(
			"failed to reconcile FargateProfile %s/%s: %v (ensure vpc-subnet-ids are private subnets and satisfy EKS Fargate requirements)",
			cluster.Namespace,
			corednsFargateObjName,
			err,
		)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonAWSPrerequisitesNotReady, msg)
		return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
	} else if !ok {
		msg := fmt.Sprintf("FargateProfile %s/%s exists and is not managed by %s", cluster.Namespace, corednsFargateObjName, karpenterManagedByValue)
		r.emitEvent(cluster, corev1.EventTypeWarning, reasonResourceOwnership, msg)
		return ctrl.Result{RequeueAfter: r.steadyStateRequeue()}, nil
	}

	karpenterFargateActive, err := r.isACKFargateProfileActive(ctx, cluster.Namespace, karpenterFargateObjName)
	if err != nil {
		return ctrl.Result{}, err
	}
	corednsFargateActive, err := r.isACKFargateProfileActive(ctx, cluster.Namespace, corednsFargateObjName)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !karpenterFargateActive || !corednsFargateActive {
		requeueAfter = r.failureBackoff()
	}

	// 5) Flux: install Karpenter chart to workload cluster via remote kubeconfig.
	fluxInstalled, err := r.ensureFluxKarpenter(ctx, cluster, capiClusterName, eksClusterName, endpoint, controllerRoleARN)
	if err != nil {
		if isNoMatchError(err) {
			msg := "Flux (source-controller/helm-controller) is not installed in the management cluster"
			r.emitEvent(cluster, corev1.EventTypeWarning, reasonFluxNotInstalled, msg)
			// Keep reconciling AWS resources even without Flux.
			fluxInstalled = false
		} else {
			return ctrl.Result{}, err
		}
	}

	// 6) ClusterResourceSet: apply default NodePool/EC2NodeClass to workload cluster.
	if len(securityGroupIDs) > 0 {
		if err := r.ensureDefaultNodePoolResources(ctx, cluster, capiClusterName, eksClusterName, nodeInstanceProfileAWSName, subnetIDs, securityGroupIDs); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 7) Workload: restart Pods that were created before FargateProfile became ACTIVE.
	if needs, err := r.ensureWorkloadRolloutRestarts(ctx, cluster, capiClusterName, corednsFargateActive, karpenterFargateActive, fluxInstalled); err != nil {
		log.Error(err, "failed to rollout-restart workload deployments", "cluster", req.String())
		requeueAfter = r.failureBackoff()
	} else if needs {
		requeueAfter = r.failureBackoff()
	}

	log.V(1).Info(
		"reconciled EKS Karpenter bootstrap",
		"cluster", req.String(),
		"eksClusterName", eksClusterName,
		"ackClusterName", ackClusterName,
		"endpoint", endpoint,
		"fluxInstalled", fluxInstalled,
		"karpenterFargateActive", karpenterFargateActive,
		"corednsFargateActive", corednsFargateActive,
	)
	if requeueAfter == r.steadyStateRequeue() {
		// Event noise is undesirable; emit only when we're in steady state.
		r.emitEvent(cluster, corev1.EventTypeNormal, reasonBootstrapperReconciled, "karpenter bootstrap resources reconciled")
	}
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func (r *EKSKarpenterBootstrapperReconciler) reconcileDeletingCluster(ctx context.Context, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	if r == nil || cluster == nil {
		return ctrl.Result{}, nil
	}
	if !isKarpenterEnabled(cluster) {
		return ctrl.Result{}, nil
	}

	capiClusterName, eksClusterName, ackClusterName := resolveClusterNames(cluster)
	log := logf.FromContext(ctx)

	// Resolve region from the Cluster annotation first. ACK Cluster may already be deleting/not-found.
	region := resolveRegion(cluster, nil)
	if strings.TrimSpace(region) == "" {
		if secretRegion, secretEKSName, ok, err := r.readWorkloadKubeconfigMetadata(ctx, cluster.Namespace, capiClusterName); err != nil {
			log.Error(err, "failed to read workload kubeconfig secret metadata", "cluster", client.ObjectKeyFromObject(cluster).String())
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
		log.Error(err, "failed to stop workload Karpenter provisioning", "cluster", client.ObjectKeyFromObject(cluster).String(), "eksClusterName", eksClusterName)
	}

	if strings.TrimSpace(region) == "" {
		log.Info("skip Karpenter EC2 node cleanup (region not resolved)", "cluster", client.ObjectKeyFromObject(cluster).String())
		return ctrl.Result{}, nil
	}
	if strings.TrimSpace(eksClusterName) == "" {
		log.Info("skip Karpenter EC2 node cleanup (EKS cluster name not resolved)", "cluster", client.ObjectKeyFromObject(cluster).String())
		return ctrl.Result{}, nil
	}

	// Terminate instances, with a few retries to reduce the chance of replacement nodes.
	var (
		lastDone        bool
		lastToTerminate int
		lastShutting    int
	)
	for attempt := 0; attempt < 3; attempt++ {
		done, toTerminate, shuttingDown, err := cleanupKarpenterEC2Instances(ctx, region, eksClusterName)
		if err != nil {
			log.Error(err, "failed to cleanup Karpenter EC2 instances", "cluster", client.ObjectKeyFromObject(cluster).String(), "eksClusterName", eksClusterName)
			return ctrl.Result{RequeueAfter: r.failureBackoff()}, nil
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
		// Best-effort: the Cluster object may disappear quickly during deletion, so we avoid relying on requeue.
		log.V(1).Info(
			"Karpenter EC2 instances termination initiated",
			"cluster", client.ObjectKeyFromObject(cluster).String(),
			"eksClusterName", eksClusterName,
			"toTerminate", lastToTerminate,
			"shuttingDown", lastShutting,
		)
		return ctrl.Result{}, nil
	}
	log.V(1).Info("Karpenter EC2 instances termination complete", "cluster", client.ObjectKeyFromObject(cluster).String(), "eksClusterName", eksClusterName)
	return ctrl.Result{}, nil
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

	ackCluster := &unstructured.Unstructured{}
	ackCluster.SetGroupVersionKind(ackClusterGVK)

	return ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.Cluster{}).
		Watches(ackCluster, handler.EnqueueRequestsFromMapFunc(r.mapACKClusterToCAPIClustersForKarpenter)).
		Named("eks-karpenter-bootstrapper").
		Complete(r)
}

func (r *EKSKarpenterBootstrapperReconciler) mapACKClusterToCAPIClustersForKarpenter(ctx context.Context, obj client.Object) []reconcile.Request {
	namespace := obj.GetNamespace()
	ackName := obj.GetName()

	clusters := &clusterv1.ClusterList{}
	if err := r.List(ctx, clusters, client.InNamespace(namespace)); err != nil {
		logf.FromContext(ctx).Error(err, "list CAPI clusters for ACK mapping", "namespace", namespace, "ackCluster", ackName)
		return nil
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

func (r *EKSKarpenterBootstrapperReconciler) ensureOIDCProvider(ctx context.Context, owner *clusterv1.Cluster, name, region, issuerURL string) (bool, error) {
	obj := newUnstructured(ackOIDCProviderGVK, owner.Namespace, name)
	return r.upsertManagedUnstructured(ctx, owner, obj, func(u *unstructured.Unstructured) error {
		setRegionAnnotation(u, region)
		setManagedBy(u)
		setClusterLabel(u, owner.Name)
		mustSetNestedString(u, issuerURL, "spec", "url")
		// ACK IAM OpenIDConnectProvider uses spec.clientIDs.
		mustSetNestedSlice(u, []any{"sts.amazonaws.com"}, "spec", "clientIDs")
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
		return nil
	})
}

func (r *EKSKarpenterBootstrapperReconciler) ensureIAMRoleForFargatePods(ctx context.Context, owner *clusterv1.Cluster, name, region, awsRoleName string, managedPolicyARNs []string) (bool, error) {
	obj := newUnstructured(ackIAMRoleGVK, owner.Namespace, name)
	assume := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"eks-fargate-pods.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
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
		return nil
	})
}

func (r *EKSKarpenterBootstrapperReconciler) ensureFargateProfile(ctx context.Context, owner *clusterv1.Cluster, name, region, ackClusterName, profileName, podExecutionRoleRefName string, subnetIDs []string, selectors []map[string]any) (bool, error) {
	obj := newUnstructured(ackFargateProfileGVK, owner.Namespace, name)
	return r.upsertManagedUnstructured(ctx, owner, obj, func(u *unstructured.Unstructured) error {
		setRegionAnnotation(u, region)
		setManagedBy(u)
		setClusterLabel(u, owner.Name)
		mustSetNestedString(u, profileName, "spec", "name")
		mustSetNestedField(u, map[string]any{"from": map[string]any{"name": ackClusterName, "namespace": owner.Namespace}}, "spec", "clusterRef")
		mustSetNestedField(u, map[string]any{"from": map[string]any{"name": podExecutionRoleRefName, "namespace": owner.Namespace}}, "spec", "podExecutionRoleRef")
		subnetsAny := []any{}
		for _, s := range subnetIDs {
			subnetsAny = append(subnetsAny, s)
		}
		mustSetNestedSlice(u, subnetsAny, "spec", "subnets")
		selAny := []any{}
		for _, sel := range selectors {
			selAny = append(selAny, sel)
		}
		mustSetNestedSlice(u, selAny, "spec", "selectors")
		return nil
	})
}

func (r *EKSKarpenterBootstrapperReconciler) ensureNodeSecurityGroup(ctx context.Context, owner *clusterv1.Cluster, region, eksClusterName string, subnetIDs []string) (string, bool, error) {
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
	if len(subnetIDs) == 0 {
		return "", false, fmt.Errorf("subnet IDs are empty")
	}

	vpcID, vpcCIDRs, err := discoverVPCBySubnets(ctx, region, subnetIDs)
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
		tags := []any{
			map[string]any{"key": "Name", "value": awsName},
			map[string]any{"key": "karpenter.sh/discovery", "value": eksClusterName},
			map[string]any{"key": fmt.Sprintf("kubernetes.io/cluster/%s", eksClusterName), "value": "owned"},
		}
		mustSetNestedSlice(u, tags, "spec", "tags")
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

func discoverVPCBySubnets(ctx context.Context, region string, subnetIDs []string) (string, []string, error) {
	region = strings.TrimSpace(region)
	if region == "" {
		return "", nil, fmt.Errorf("region is empty")
	}
	ids := []string{}
	for _, id := range subnetIDs {
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

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (r *EKSKarpenterBootstrapperReconciler) ensureFluxKarpenter(ctx context.Context, owner *clusterv1.Cluster, capiClusterName, eksClusterName, endpoint, controllerRoleARN string) (bool, error) {
	// OCIRepository
	ociRepoName := fmt.Sprintf("%s-karpenter", capiClusterName)
	oci := newUnstructured(fluxOCIRepositoryGVK, owner.Namespace, ociRepoName)
	if ok, err := r.upsertManagedUnstructured(ctx, owner, oci, func(u *unstructured.Unstructured) error {
		setManagedBy(u)
		setClusterLabel(u, owner.Name)
		mustSetNestedString(u, "10m", "spec", "interval")
		mustSetNestedString(u, "oci://public.ecr.aws/karpenter/karpenter", "spec", "url")
		mustSetNestedField(u, map[string]any{"tag": defaultKarpenterChartVersion}, "spec", "ref")
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
		values := map[string]any{
			"dnsPolicy":         "Default",
			"priorityClassName": "system-cluster-critical",
			"webhook":           map[string]any{"enabled": false},
			"settings":          map[string]any{"clusterName": eksClusterName, "clusterEndpoint": endpoint},
			"serviceAccount":    map[string]any{"annotations": map[string]any{"eks.amazonaws.com/role-arn": controllerRoleARN}},
		}
		mustSetNestedField(u, values, "spec", "values")
		return nil
	})
	if err != nil {
		return false, err
	}
	return ok, nil
}

func (r *EKSKarpenterBootstrapperReconciler) ensureDefaultNodePoolResources(ctx context.Context, owner *clusterv1.Cluster, capiClusterName, eksClusterName, nodeInstanceProfileName string, subnetIDs, securityGroupIDs []string) error {
	desiredYAML := buildDefaultNodePoolYAML(eksClusterName, nodeInstanceProfileName, subnetIDs, securityGroupIDs)

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
		if !isManagedByBootstrapper(cm.GetAnnotations()) {
			msg := fmt.Sprintf("configmap %s/%s exists and is not managed by %s", owner.Namespace, cmName, karpenterManagedByValue)
			r.emitEvent(owner, corev1.EventTypeWarning, reasonResourceOwnership, msg)
			return nil
		}
		before := cm.DeepCopy()
		mutateManagedConfigMap(cm, owner, desiredYAML)
		if err := controllerutil.SetOwnerReference(owner, cm, r.Scheme); err != nil {
			return err
		}
		if !equality.Semantic.DeepEqual(before, cm) {
			if err := r.Update(ctx, cm); err != nil {
				return err
			}
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
	}
	return nil
}

func buildDefaultNodePoolYAML(eksClusterName, nodeInstanceProfileName string, subnetIDs, securityGroupIDs []string) string {
	// EC2NodeClass (v1)
	subnetTerms := []string{}
	for _, id := range subnetIDs {
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
		if err := controllerutil.SetOwnerReference(owner, obj, r.Scheme); err != nil {
			return false, err
		}
		if err := r.Create(ctx, obj); err != nil {
			return false, err
		}
		return true, nil
	}

	if !isManagedByBootstrapper(existing.GetAnnotations()) {
		return false, nil
	}

	before := existing.DeepCopy()
	if err := mutate(existing); err != nil {
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
		panic(err)
	}
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
