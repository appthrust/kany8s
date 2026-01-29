/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controlplane

import (
	"context"
	"fmt"
	"net/url"

	controlplanev1alpha1 "github.com/reoring/kany8s/api/v1alpha1"
	"github.com/reoring/kany8s/internal/constants"
	"github.com/reoring/kany8s/internal/kubeconfig"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/util"
	capicerts "sigs.k8s.io/cluster-api/util/certs"
	capikubeconfig "sigs.k8s.io/cluster-api/util/kubeconfig"
	capisecret "sigs.k8s.io/cluster-api/util/secret"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	conditionTypeOwnerClusterResolved = "OwnerClusterResolved"
	conditionTypeReady                = "Ready"
	conditionTypeCreating             = "Creating"

	reasonOwnerClusterNotSet    = "OwnerClusterNotSet"
	reasonOwnerClusterNotFound  = "OwnerClusterNotFound"
	reasonOwnerClusterResolved  = "OwnerClusterResolved"
	reasonOwnerClusterGetFailed = "OwnerClusterGetFailed"

	reasonWaitingForInfrastructureEndpoint    = "WaitingForInfrastructureEndpoint"
	reasonWaitingForControlPlaneMachineReady  = "WaitingForControlPlaneMachineReady"
	reasonWaitingForControlPlaneMachineCreate = "WaitingForControlPlaneMachineCreated"
	reasonControlPlaneReady                   = "Ready"
)

// Kany8sKubeadmControlPlaneReconciler reconciles a Kany8sKubeadmControlPlane object
type Kany8sKubeadmControlPlaneReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kany8skubeadmcontrolplanes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kany8skubeadmcontrolplanes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kany8skubeadmcontrolplanes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// MVP behavior (self-managed bootstrap): resolve the owner Cluster (via
// OwnerReferences) and surface resolution state via Conditions/Events.
func (r *Kany8sKubeadmControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	cp := &controlplanev1alpha1.Kany8sKubeadmControlPlane{}
	if err := r.Get(ctx, req.NamespacedName, cp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	owner, err := util.GetOwnerCluster(ctx, r.Client, cp.ObjectMeta)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("owner Cluster not found yet")
			if err := r.reconcileOwnerClusterResolvedCondition(ctx, cp, metav1.ConditionFalse, reasonOwnerClusterNotFound, err.Error(), corev1.EventTypeNormal); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: constants.ControlPlaneNotReadyRequeueAfter}, nil
		}

		log.Error(err, "get owner Cluster")
		message := fmt.Sprintf("get owner Cluster: %v", err)
		if err := r.reconcileOwnerClusterResolvedCondition(ctx, cp, metav1.ConditionFalse, reasonOwnerClusterGetFailed, message, corev1.EventTypeWarning); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: constants.ControlPlaneNotReadyRequeueAfter}, nil
	}
	if owner == nil {
		message := "waiting for owner Cluster reference to be set"
		if err := r.reconcileOwnerClusterResolvedCondition(ctx, cp, metav1.ConditionFalse, reasonOwnerClusterNotSet, message, corev1.EventTypeNormal); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: constants.ControlPlaneNotReadyRequeueAfter}, nil
	}

	if err := r.reconcileOwnerClusterResolvedCondition(ctx, cp, metav1.ConditionTrue, reasonOwnerClusterResolved, "owner Cluster resolved", corev1.EventTypeNormal); err != nil {
		return ctrl.Result{}, err
	}

	var clusterConfig *bootstrapv1.ClusterConfiguration
	if cp.Spec.KubeadmConfigSpec != nil {
		clusterConfig = &cp.Spec.KubeadmConfigSpec.ClusterConfiguration
	}
	certificates := capisecret.NewCertificatesForInitialControlPlane(clusterConfig)
	clusterKey := client.ObjectKey{Name: owner.Name, Namespace: owner.Namespace}
	clusterOwnerRef := metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       owner.Name,
		UID:        owner.UID,
	}
	if err := certificates.LookupOrGenerate(ctx, r.Client, clusterKey, clusterOwnerRef); err != nil {
		log.Error(err, "reconcile cluster certificates")
		return ctrl.Result{RequeueAfter: constants.ControlPlaneNotReadyRequeueAfter}, nil
	}

	if !owner.Spec.InfrastructureRef.IsDefined() {
		log.V(1).Info("owner Cluster infrastructureRef not set yet")
		return ctrl.Result{RequeueAfter: constants.ControlPlaneNotReadyRequeueAfter}, nil
	}

	infra, err := external.GetObjectFromContractVersionedRef(ctx, r.Client, owner.Spec.InfrastructureRef, owner.Namespace)
	if err != nil {
		log.Error(err, "get infrastructure cluster")
		return ctrl.Result{RequeueAfter: constants.ControlPlaneNotReadyRequeueAfter}, nil
	}

	host, foundHost, err := unstructured.NestedString(infra.Object, "spec", "controlPlaneEndpoint", "host")
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("read infrastructure spec.controlPlaneEndpoint.host: %w", err)
	}
	port, foundPort, err := unstructured.NestedInt64(infra.Object, "spec", "controlPlaneEndpoint", "port")
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("read infrastructure spec.controlPlaneEndpoint.port: %w", err)
	}
	if !foundHost || host == "" || !foundPort || port <= 0 {
		log.V(1).Info("infrastructure controlPlaneEndpoint not set yet")
		return r.requeueNotReady(ctx, cp, reasonWaitingForInfrastructureEndpoint, "waiting for infrastructure controlPlaneEndpoint to be set")
	}

	desired := clusterv1.APIEndpoint{Host: host, Port: int32(port)}
	if cp.Spec.ControlPlaneEndpoint != desired {
		before := cp.DeepCopy()
		cp.Spec.ControlPlaneEndpoint = desired
		if err := r.Patch(ctx, cp, client.MergeFrom(before)); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileClusterKubeconfigSecret(ctx, owner, clusterOwnerRef, desired); err != nil {
		log.Error(err, "reconcile cluster kubeconfig secret")
		return ctrl.Result{RequeueAfter: constants.ControlPlaneNotReadyRequeueAfter}, nil
	}

	if err := r.reconcileInitialControlPlaneKubeadmConfig(ctx, cp, owner, clusterOwnerRef, desired, certificates); err != nil {
		log.Error(err, "reconcile initial control plane KubeadmConfig")
		return ctrl.Result{RequeueAfter: constants.ControlPlaneNotReadyRequeueAfter}, nil
	}

	infraRef, err := r.reconcileInfraMachine(ctx, cp, owner, clusterOwnerRef)
	if err != nil {
		log.Error(err, "reconcile infra machine")
		return ctrl.Result{RequeueAfter: constants.ControlPlaneNotReadyRequeueAfter}, nil
	}

	if err := r.reconcileControlPlaneMachine(ctx, cp, owner, clusterOwnerRef, infraRef); err != nil {
		log.Error(err, "reconcile control plane Machine")
		return ctrl.Result{RequeueAfter: constants.ControlPlaneNotReadyRequeueAfter}, nil
	}

	return r.reconcileReadiness(ctx, cp, owner)
}

func (r *Kany8sKubeadmControlPlaneReconciler) reconcileReadiness(ctx context.Context, cp *controlplanev1alpha1.Kany8sKubeadmControlPlane, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	if cp == nil {
		return ctrl.Result{}, fmt.Errorf("control plane is nil")
	}
	if cluster == nil {
		return ctrl.Result{}, fmt.Errorf("cluster is nil")
	}

	endpointResolved := cp.Spec.ControlPlaneEndpoint.Host != "" && cp.Spec.ControlPlaneEndpoint.Port > 0
	if !endpointResolved {
		return r.requeueNotReady(ctx, cp, reasonWaitingForInfrastructureEndpoint, "waiting for infrastructure controlPlaneEndpoint to be set")
	}

	machineReady, err := r.hasReadyControlPlaneMachine(ctx, cluster)
	if err != nil {
		message := fmt.Sprintf("list control plane Machines: %v", err)
		return r.requeueNotReady(ctx, cp, reasonWaitingForControlPlaneMachineCreate, message)
	}

	if machineReady && !cp.Status.Initialization.ControlPlaneInitialized {
		if err := r.reconcileControlPlaneInitialized(ctx, cp); err != nil {
			return ctrl.Result{}, err
		}
	}
	if !cp.Status.Initialization.ControlPlaneInitialized {
		return r.requeueNotReady(ctx, cp, reasonWaitingForControlPlaneMachineReady, "waiting for a control plane Machine to become Ready")
	}

	if err := r.reconcileReadyConditions(ctx, cp, metav1.ConditionTrue, reasonControlPlaneReady, "control plane is ready"); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *Kany8sKubeadmControlPlaneReconciler) requeueNotReady(ctx context.Context, cp *controlplanev1alpha1.Kany8sKubeadmControlPlane, reason string, message string) (ctrl.Result, error) {
	if err := r.reconcileReadyConditions(ctx, cp, metav1.ConditionFalse, reason, message); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: constants.ControlPlaneNotReadyRequeueAfter}, nil
}

func (r *Kany8sKubeadmControlPlaneReconciler) hasReadyControlPlaneMachine(ctx context.Context, cluster *clusterv1.Cluster) (bool, error) {
	if cluster == nil {
		return false, fmt.Errorf("cluster is nil")
	}
	if cluster.Name == "" {
		return false, fmt.Errorf("cluster name is empty")
	}

	var machines clusterv1.MachineList
	if err := r.List(ctx, &machines,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels{
			clusterv1.ClusterNameLabel:         cluster.Name,
			clusterv1.MachineControlPlaneLabel: "true",
		},
	); err != nil {
		return false, err
	}

	for i := range machines.Items {
		m := &machines.Items[i]
		if meta.IsStatusConditionTrue(m.Status.Conditions, conditionTypeReady) {
			return true, nil
		}
	}
	return false, nil
}

func (r *Kany8sKubeadmControlPlaneReconciler) reconcileControlPlaneInitialized(ctx context.Context, cp *controlplanev1alpha1.Kany8sKubeadmControlPlane) error {
	if cp == nil {
		return fmt.Errorf("control plane is nil")
	}
	if cp.Status.Initialization.ControlPlaneInitialized {
		return nil
	}

	before := cp.DeepCopy()
	cp.Status.Initialization.ControlPlaneInitialized = true
	return r.Status().Patch(ctx, cp, client.MergeFrom(before))
}

func (r *Kany8sKubeadmControlPlaneReconciler) reconcileReadyConditions(
	ctx context.Context,
	cp *controlplanev1alpha1.Kany8sKubeadmControlPlane,
	status metav1.ConditionStatus,
	reason string,
	message string,
) error {
	if cp == nil {
		return fmt.Errorf("control plane is nil")
	}

	before := cp.DeepCopy()

	readyCond := metav1.Condition{
		Type:               conditionTypeReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: cp.Generation,
	}
	meta.SetStatusCondition(&cp.Status.Conditions, readyCond)

	if status == metav1.ConditionTrue {
		meta.RemoveStatusCondition(&cp.Status.Conditions, conditionTypeCreating)
		if cp.Spec.Version != "" && cp.Status.Version != cp.Spec.Version {
			cp.Status.Version = cp.Spec.Version
		}
	} else {
		creatingCond := metav1.Condition{
			Type:               conditionTypeCreating,
			Status:             metav1.ConditionTrue,
			Reason:             reason,
			Message:            message,
			ObservedGeneration: cp.Generation,
		}
		meta.SetStatusCondition(&cp.Status.Conditions, creatingCond)
	}

	cp.Status.FailureReason = nil
	cp.Status.FailureMessage = nil

	return r.Status().Patch(ctx, cp, client.MergeFrom(before))
}

func (r *Kany8sKubeadmControlPlaneReconciler) reconcileInitialControlPlaneKubeadmConfig(
	ctx context.Context,
	cp *controlplanev1alpha1.Kany8sKubeadmControlPlane,
	cluster *clusterv1.Cluster,
	clusterOwnerRef metav1.OwnerReference,
	endpoint clusterv1.APIEndpoint,
	certificates capisecret.Certificates,
) error {
	if cp == nil {
		return fmt.Errorf("control plane is nil")
	}
	if cluster == nil {
		return fmt.Errorf("cluster is nil")
	}
	if endpoint.Host == "" || endpoint.Port <= 0 {
		return fmt.Errorf("control plane endpoint is not set")
	}

	name := fmt.Sprintf("%s-control-plane-0", cluster.Name)
	key := client.ObjectKey{Name: name, Namespace: cluster.Namespace}

	desiredSpec := bootstrapv1.KubeadmConfigSpec{}
	if cp.Spec.KubeadmConfigSpec != nil {
		desiredSpec = *cp.Spec.KubeadmConfigSpec.DeepCopy()
	}
	desiredSpec.ClusterConfiguration.ControlPlaneEndpoint = endpoint.String()
	if desiredSpec.InitConfiguration.LocalAPIEndpoint.BindPort == 0 {
		desiredSpec.InitConfiguration.LocalAPIEndpoint.BindPort = endpoint.Port
	}
	desiredSpec.Files = mergeBootstrapFiles(desiredSpec.Files, certificates.AsFiles())

	existing := &bootstrapv1.KubeadmConfig{}
	if err := r.Get(ctx, key, existing); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}

		created := &bootstrapv1.KubeadmConfig{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cluster.Namespace}}
		created.Labels = map[string]string{clusterv1.ClusterNameLabel: cluster.Name}
		created.OwnerReferences = []metav1.OwnerReference{clusterOwnerRef}
		created.Spec = desiredSpec
		if err := r.Create(ctx, created); err != nil {
			if apierrors.IsAlreadyExists(err) {
				return nil
			}
			return err
		}
		return nil
	}

	before := existing.DeepCopy()
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	existing.Labels[clusterv1.ClusterNameLabel] = cluster.Name
	existing.OwnerReferences = []metav1.OwnerReference{clusterOwnerRef}
	existing.Spec = desiredSpec
	return r.Patch(ctx, existing, client.MergeFrom(before))
}

func mergeBootstrapFiles(base []bootstrapv1.File, inject []bootstrapv1.File) []bootstrapv1.File {
	if len(inject) == 0 {
		return base
	}

	overrides := make(map[string]struct{}, len(inject))
	for _, f := range inject {
		if f.Path == "" {
			continue
		}
		overrides[f.Path] = struct{}{}
	}

	out := make([]bootstrapv1.File, 0, len(base)+len(inject))
	for _, f := range base {
		if _, ok := overrides[f.Path]; ok {
			continue
		}
		out = append(out, f)
	}
	out = append(out, inject...)
	return out
}

func (r *Kany8sKubeadmControlPlaneReconciler) reconcileInfraMachine(ctx context.Context, cp *controlplanev1alpha1.Kany8sKubeadmControlPlane, cluster *clusterv1.Cluster, clusterOwnerRef metav1.OwnerReference) (clusterv1.ContractVersionedObjectReference, error) {
	if cp == nil {
		return clusterv1.ContractVersionedObjectReference{}, fmt.Errorf("control plane is nil")
	}
	if cluster == nil {
		return clusterv1.ContractVersionedObjectReference{}, fmt.Errorf("cluster is nil")
	}

	if !cp.Spec.MachineTemplate.InfrastructureRef.IsDefined() {
		return clusterv1.ContractVersionedObjectReference{}, fmt.Errorf("machineTemplate.infrastructureRef is not set")
	}

	machineTemplate, err := external.GetObjectFromContractVersionedRef(ctx, r.Client, cp.Spec.MachineTemplate.InfrastructureRef, cluster.Namespace)
	if err != nil {
		return clusterv1.ContractVersionedObjectReference{}, err
	}
	if machineTemplate == nil {
		return clusterv1.ContractVersionedObjectReference{}, fmt.Errorf("machineTemplate.infrastructureRef resolved to nil")
	}

	infraMachineName := fmt.Sprintf("%s-control-plane-0", cluster.Name)
	generatedInfraMachine, err := external.GenerateTemplate(&external.GenerateTemplateInput{
		Template: machineTemplate,
		TemplateRef: &corev1.ObjectReference{
			APIVersion: machineTemplate.GetAPIVersion(),
			Kind:       machineTemplate.GetKind(),
			Namespace:  cluster.Namespace,
			Name:       machineTemplate.GetName(),
		},
		Namespace:   cluster.Namespace,
		Name:        infraMachineName,
		ClusterName: cluster.Name,
	})
	if err != nil {
		return clusterv1.ContractVersionedObjectReference{}, err
	}
	if generatedInfraMachine == nil {
		return clusterv1.ContractVersionedObjectReference{}, fmt.Errorf("generated infra machine is nil")
	}
	generatedInfraMachine.SetOwnerReferences([]metav1.OwnerReference{clusterOwnerRef})

	infraGVK := generatedInfraMachine.GroupVersionKind()
	infraRef := clusterv1.ContractVersionedObjectReference{APIGroup: infraGVK.Group, Kind: infraGVK.Kind, Name: infraMachineName}

	existing := &unstructured.Unstructured{}
	existing.SetAPIVersion(generatedInfraMachine.GetAPIVersion())
	existing.SetKind(generatedInfraMachine.GetKind())
	key := client.ObjectKey{Name: infraMachineName, Namespace: cluster.Namespace}
	if err := r.Get(ctx, key, existing); err != nil {
		if !apierrors.IsNotFound(err) {
			return clusterv1.ContractVersionedObjectReference{}, err
		}
		if err := r.Create(ctx, generatedInfraMachine); err != nil {
			if apierrors.IsAlreadyExists(err) {
				return infraRef, nil
			}
			return clusterv1.ContractVersionedObjectReference{}, err
		}
		return infraRef, nil
	}

	return infraRef, nil
}

func (r *Kany8sKubeadmControlPlaneReconciler) reconcileControlPlaneMachine(
	ctx context.Context,
	cp *controlplanev1alpha1.Kany8sKubeadmControlPlane,
	cluster *clusterv1.Cluster,
	clusterOwnerRef metav1.OwnerReference,
	infraRef clusterv1.ContractVersionedObjectReference,
) error {
	if cp == nil {
		return fmt.Errorf("control plane is nil")
	}
	if cluster == nil {
		return fmt.Errorf("cluster is nil")
	}
	if cluster.Name == "" {
		return fmt.Errorf("cluster name is empty")
	}
	if !infraRef.IsDefined() {
		return fmt.Errorf("infrastructureRef is not set")
	}

	name := fmt.Sprintf("%s-control-plane-0", cluster.Name)
	key := client.ObjectKey{Name: name, Namespace: cluster.Namespace}

	bootstrapRef := clusterv1.ContractVersionedObjectReference{APIGroup: bootstrapv1.GroupVersion.Group, Kind: "KubeadmConfig", Name: name}

	existing := &clusterv1.Machine{}
	if err := r.Get(ctx, key, existing); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}

		created := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cluster.Namespace}}
		created.Labels = map[string]string{
			clusterv1.ClusterNameLabel:         cluster.Name,
			clusterv1.MachineControlPlaneLabel: "true",
		}
		created.OwnerReferences = []metav1.OwnerReference{clusterOwnerRef}
		created.Spec.ClusterName = cluster.Name
		created.Spec.Version = cp.Spec.Version
		created.Spec.Bootstrap = clusterv1.Bootstrap{ConfigRef: bootstrapRef}
		created.Spec.InfrastructureRef = infraRef
		if err := r.Create(ctx, created); err != nil {
			if apierrors.IsAlreadyExists(err) {
				return nil
			}
			return err
		}
		return nil
	}

	before := existing.DeepCopy()
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	existing.Labels[clusterv1.ClusterNameLabel] = cluster.Name
	existing.Labels[clusterv1.MachineControlPlaneLabel] = "true"
	existing.OwnerReferences = []metav1.OwnerReference{clusterOwnerRef}
	existing.Spec.ClusterName = cluster.Name
	existing.Spec.Version = cp.Spec.Version
	existing.Spec.Bootstrap.ConfigRef = bootstrapRef
	existing.Spec.InfrastructureRef = infraRef

	return r.Patch(ctx, existing, client.MergeFrom(before))
}

func (r *Kany8sKubeadmControlPlaneReconciler) reconcileClusterKubeconfigSecret(ctx context.Context, cluster *clusterv1.Cluster, clusterOwnerRef metav1.OwnerReference, endpoint clusterv1.APIEndpoint) error {
	if cluster == nil {
		return fmt.Errorf("cluster is nil")
	}

	secretName, err := kubeconfig.SecretName(cluster.Name)
	if err != nil {
		return err
	}

	endpointStr := endpoint.String()
	if endpointStr == "" {
		return fmt.Errorf("control plane endpoint is empty")
	}
	server, err := url.JoinPath("https://", endpointStr)
	if err != nil {
		return err
	}

	key := client.ObjectKey{Name: secretName, Namespace: cluster.Namespace}
	secretObj := &corev1.Secret{}
	getErr := r.Get(ctx, key, secretObj)
	if getErr != nil && !apierrors.IsNotFound(getErr) {
		return getErr
	}

	needsKubeconfig := apierrors.IsNotFound(getErr)
	if !needsKubeconfig {
		kc, ok := secretObj.Data[kubeconfig.DataKey]
		if !ok || len(kc) == 0 {
			needsKubeconfig = true
		} else {
			clientConfig, err := clientcmd.NewClientConfigFromBytes(kc)
			if err != nil {
				needsKubeconfig = true
			} else if restCfg, err := clientConfig.ClientConfig(); err != nil {
				needsKubeconfig = true
			} else if restCfg.Host != server {
				needsKubeconfig = true
			}
		}
	}

	var out []byte
	if needsKubeconfig {
		caSecret := &corev1.Secret{}
		caKey := client.ObjectKey{Name: capisecret.Name(cluster.Name, capisecret.ClusterCA), Namespace: cluster.Namespace}
		if err := r.Get(ctx, caKey, caSecret); err != nil {
			return err
		}

		cert, err := capicerts.DecodeCertPEM(caSecret.Data[capisecret.TLSCrtDataName])
		if err != nil {
			return fmt.Errorf("decode cluster CA cert: %w", err)
		}
		if cert == nil {
			return fmt.Errorf("cluster CA cert not found")
		}
		key, err := capicerts.DecodePrivateKeyPEM(caSecret.Data[capisecret.TLSKeyDataName])
		if err != nil {
			return fmt.Errorf("decode cluster CA private key: %w", err)
		}
		if key == nil {
			return fmt.Errorf("cluster CA private key not found")
		}

		cfg, err := capikubeconfig.New(cluster.Name, server, cert, key)
		if err != nil {
			return fmt.Errorf("generate kubeconfig: %w", err)
		}
		out, err = clientcmd.Write(*cfg)
		if err != nil {
			return fmt.Errorf("serialize kubeconfig: %w", err)
		}
	}

	if apierrors.IsNotFound(getErr) {
		created, err := kubeconfig.NewSecret(cluster.Name, cluster.Namespace, out)
		if err != nil {
			return err
		}
		created.OwnerReferences = []metav1.OwnerReference{clusterOwnerRef}
		if err := r.Create(ctx, created); err != nil {
			if apierrors.IsAlreadyExists(err) {
				return nil
			}
			return err
		}
		return nil
	}

	before := secretObj.DeepCopy()
	if secretObj.Labels == nil {
		secretObj.Labels = map[string]string{}
	}
	secretObj.Labels[kubeconfig.ClusterNameLabelKey] = cluster.Name
	secretObj.Type = kubeconfig.SecretType
	secretObj.OwnerReferences = []metav1.OwnerReference{clusterOwnerRef}
	if secretObj.Data == nil {
		secretObj.Data = map[string][]byte{}
	}
	if needsKubeconfig {
		secretObj.Data[kubeconfig.DataKey] = out
	}
	return r.Patch(ctx, secretObj, client.MergeFrom(before))
}

func (r *Kany8sKubeadmControlPlaneReconciler) reconcileOwnerClusterResolvedCondition(
	ctx context.Context,
	cp *controlplanev1alpha1.Kany8sKubeadmControlPlane,
	status metav1.ConditionStatus,
	reason string,
	message string,
	eventType string,
) error {
	if cp == nil {
		return fmt.Errorf("control plane is nil")
	}

	cond := metav1.Condition{
		Type:               conditionTypeOwnerClusterResolved,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: cp.Generation,
	}

	prev := meta.FindStatusCondition(cp.Status.Conditions, cond.Type)
	shouldEmit := prev == nil || prev.Status != cond.Status || prev.Reason != cond.Reason || prev.Message != cond.Message

	before := cp.DeepCopy()
	meta.SetStatusCondition(&cp.Status.Conditions, cond)
	if err := r.Status().Patch(ctx, cp, client.MergeFrom(before)); err != nil {
		return err
	}

	if shouldEmit && r.Recorder != nil {
		r.Recorder.Event(cp, eventType, reason, message)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Kany8sKubeadmControlPlaneReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&controlplanev1alpha1.Kany8sKubeadmControlPlane{}).
		Named("controlplane-kany8skubeadmcontrolplane").
		Complete(r)
}
