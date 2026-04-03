#!/usr/bin/env bash
# create-eks-fullstack.sh — EKS + Karpenter フルセット作成
#
# Usage: bash hack/create-eks-fullstack.sh
#
# 前提: setup-eks-management.sh が完了していること (Kind + CAPI + kro + ACK + Kany8s)
#
# 作成されるもの:
#   - VPC + IGW + NAT Gateway + Public/Private subnets (ACK EC2)
#   - EKS Control Plane (ACK EKS via kro)
#   - kubeconfig-rotator plugin
#   - Flux (HelmRelease 用)
#   - karpenter-bootstrapper plugin → Fargate → Karpenter → EC2 nodes
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

# ─── kubectl ARG_MAX workaround ─────────────────────────
_real_kubectl="$(command -v kubectl)"
kubectl() {
    env -i \
        HOME="${HOME}" \
        PATH="${PATH}" \
        KUBECONFIG="${KUBECONFIG:-}" \
        USER="${USER:-}" \
        TERM="${TERM:-dumb}" \
        "${_real_kubectl}" "$@"
}
export -f kubectl 2>/dev/null || true

log() { echo "[fullstack] $(date +%H:%M:%S) $*"; }

# ─── 設定 ───────────────────────────────────────────────
AWS_REGION="${AWS_REGION:-ap-northeast-1}"
CLUSTER_NAME="${CLUSTER_NAME:-eks-full-$(date +%Y%m%d%H%M%S)}"
NAMESPACE="${NAMESPACE:-default}"
KUBERNETES_VERSION="${KUBERNETES_VERSION:-v1.35.0}"
EKS_VERSION="${EKS_VERSION:-1.35}"
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-kany8s-eks}"

# Network
NETWORK_NAME="${NETWORK_NAME:-${CLUSTER_NAME}}"
VPC_CIDR="${VPC_CIDR:-10.35.0.0/16}"
PUBLIC_SUBNET_A_CIDR="${PUBLIC_SUBNET_A_CIDR:-10.35.0.0/24}"
PUBLIC_SUBNET_A_AZ="${PUBLIC_SUBNET_A_AZ:-ap-northeast-1a}"
PRIVATE_SUBNET_A_CIDR="${PRIVATE_SUBNET_A_CIDR:-10.35.10.0/24}"
PRIVATE_SUBNET_A_AZ="${PRIVATE_SUBNET_A_AZ:-ap-northeast-1a}"
PRIVATE_SUBNET_B_CIDR="${PRIVATE_SUBNET_B_CIDR:-10.35.11.0/24}"
PRIVATE_SUBNET_B_AZ="${PRIVATE_SUBNET_B_AZ:-ap-northeast-1c}"

# EKS access
EKS_ACCESS_MODE="${EKS_ACCESS_MODE:-API_AND_CONFIG_MAP}"
EKS_ENDPOINT_PRIVATE_ACCESS="${EKS_ENDPOINT_PRIVATE_ACCESS:-true}"
EKS_ENDPOINT_PUBLIC_ACCESS="${EKS_ENDPOINT_PUBLIC_ACCESS:-true}"

# Public IP for EKS endpoint access
if [[ -z "${PUBLIC_ACCESS_CIDR:-}" ]]; then
    my_ip="$(curl -fsSL https://checkip.amazonaws.com | tr -d '\n')"
    PUBLIC_ACCESS_CIDR="${my_ip}/32"
fi

# Plugin images
ROTATOR_IMG="${ROTATOR_IMG:-example.com/eks-kubeconfig-rotator:dev}"
BOOTSTRAPPER_IMG="${BOOTSTRAPPER_IMG:-example.com/eks-karpenter-bootstrapper:dev}"

ARTIFACTS_DIR="${ARTIFACTS_DIR:-/tmp/kany8s-eks-fullstack-$(date +%Y%m%d%H%M%S)}"
mkdir -p "${ARTIFACTS_DIR}"
log_file="${ARTIFACTS_DIR}/fullstack.log"
exec > >(tee -a "${log_file}") 2>&1

log "=========================================="
log "  EKS Full-Stack Creation"
log "=========================================="
log "  CLUSTER_NAME:     ${CLUSTER_NAME}"
log "  AWS_REGION:       ${AWS_REGION}"
log "  EKS_VERSION:      ${EKS_VERSION}"
log "  PUBLIC_ACCESS:    ${PUBLIC_ACCESS_CIDR}"
log "  Artifacts:        ${ARTIFACTS_DIR}"
log "=========================================="

kubectl config use-context "kind-${KIND_CLUSTER_NAME}"

# ═══════════════════════════════════════════════════════════
# Phase 1: Network (VPC + IGW + NAT + Public/Private Subnets)
# ═══════════════════════════════════════════════════════════
log ""
log "Phase 1: Creating network infrastructure..."

rendered_network="${ARTIFACTS_DIR}/network.yaml"
sed \
    -e "s|__NETWORK_NAME__|${NETWORK_NAME}|g" \
    -e "s|__NAMESPACE__|${NAMESPACE}|g" \
    -e "s|__AWS_REGION__|${AWS_REGION}|g" \
    -e "s|__VPC_CIDR__|${VPC_CIDR}|g" \
    -e "s|__PUBLIC_SUBNET_A_CIDR__|${PUBLIC_SUBNET_A_CIDR}|g" \
    -e "s|__PUBLIC_SUBNET_A_AZ__|${PUBLIC_SUBNET_A_AZ}|g" \
    -e "s|__PRIVATE_SUBNET_A_CIDR__|${PRIVATE_SUBNET_A_CIDR}|g" \
    -e "s|__PRIVATE_SUBNET_A_AZ__|${PRIVATE_SUBNET_A_AZ}|g" \
    -e "s|__PRIVATE_SUBNET_B_CIDR__|${PRIVATE_SUBNET_B_CIDR}|g" \
    -e "s|__PRIVATE_SUBNET_B_AZ__|${PRIVATE_SUBNET_B_AZ}|g" \
    docs/eks/byo-network/manifests/bootstrap-network-private-nat.yaml.tpl \
    >"${rendered_network}"

kubectl apply -f "${rendered_network}"

log "Waiting for VPC..."
for i in $(seq 1 60); do
    vpc_state="$(kubectl get vpcs.ec2.services.k8s.aws "${NETWORK_NAME}-vpc" -n "${NAMESPACE}" -o jsonpath='{.status.state}' 2>/dev/null || echo "")"
    if [[ "${vpc_state}" == "available" ]]; then
        log "VPC is available."
        break
    fi
    log "  VPC state: ${vpc_state:-pending} (${i}/60)"
    sleep 5
done

log "Waiting for private subnets..."
_ec2_restarted=false
for i in $(seq 1 90); do
    sub_a_id="$(kubectl get subnets.ec2.services.k8s.aws "${NETWORK_NAME}-subnet-private-a" -n "${NAMESPACE}" -o jsonpath='{.status.subnetID}' 2>/dev/null || echo "")"
    sub_b_id="$(kubectl get subnets.ec2.services.k8s.aws "${NETWORK_NAME}-subnet-private-b" -n "${NAMESPACE}" -o jsonpath='{.status.subnetID}' 2>/dev/null || echo "")"
    if [[ -n "${sub_a_id}" && -n "${sub_b_id}" ]]; then
        log "Private subnets ready: ${sub_a_id}, ${sub_b_id}"
        break
    fi
    # Workaround: ACK EC2 controller can have stale reference cache.
    # If subnets are still pending after 2 min, restart the controller.
    if [[ "${_ec2_restarted}" == "false" ]] && (( i >= 24 )); then
        log "Subnets stuck — restarting ACK EC2 controller (stale cache workaround)..."
        kubectl -n ack-system rollout restart deploy/ack-ec2-controller-ec2-chart
        kubectl -n ack-system rollout status deploy/ack-ec2-controller-ec2-chart --timeout=120s
        _ec2_restarted=true
    fi
    if (( i % 6 == 0 )); then
        log "  Subnets: a=${sub_a_id:-pending} b=${sub_b_id:-pending} (${i}/90)"
    fi
    sleep 5
done

if [[ -z "${sub_a_id:-}" || -z "${sub_b_id:-}" ]]; then
    log "ERROR: Private subnets not ready after timeout"
    exit 1
fi

log "Waiting for NAT Gateway (this can take 1-2 min)..."
for i in $(seq 1 60); do
    nat_state="$(kubectl get natgateways.ec2.services.k8s.aws "${NETWORK_NAME}-natgw" -n "${NAMESPACE}" -o jsonpath='{.status.state}' 2>/dev/null || echo "")"
    if [[ "${nat_state}" == "available" ]]; then
        log "NAT Gateway is available."
        break
    fi
    log "  NAT state: ${nat_state:-pending} (${i}/60)"
    sleep 10
done

# ═══════════════════════════════════════════════════════════
# Phase 2: EKS Control Plane (BYO RGDs + ClusterClass)
# ═══════════════════════════════════════════════════════════
log ""
log "Phase 2: Creating EKS control plane..."

log "Applying BYO RGDs..."
kubectl apply -f docs/eks/byo-network/manifests/aws-byo-network-rgd.yaml
kubectl wait --for=condition=ResourceGraphAccepted --timeout=120s rgd/aws-byo-network.kro.run

kubectl apply -f docs/eks/byo-network/manifests/eks-control-plane-byo-rgd.yaml
kubectl wait --for=condition=ResourceGraphAccepted --timeout=120s rgd/eks-control-plane-byo.kro.run
log "RGDs accepted."

log "Applying ClusterClass..."
kubectl -n "${NAMESPACE}" apply -f docs/eks/byo-network/manifests/clusterclass-eks-byo.yaml

log "Rendering Cluster manifest..."
rendered_cluster="${ARTIFACTS_DIR}/cluster.yaml"
sed \
    -e "s|__CLUSTER_NAME__|${CLUSTER_NAME}|g" \
    -e "s|__NAMESPACE__|${NAMESPACE}|g" \
    -e "s|__KUBERNETES_VERSION__|${KUBERNETES_VERSION}|g" \
    -e "s|__AWS_REGION__|${AWS_REGION}|g" \
    -e "s|__EKS_VERSION__|${EKS_VERSION}|g" \
    -e "s|__SUBNET_ID_1__|${sub_a_id}|g" \
    -e "s|__SUBNET_ID_2__|${sub_b_id}|g" \
    -e "s|__SECURITY_GROUP_IDS_JSON__|[]|g" \
    -e "s|__PUBLIC_ACCESS_CIDR__|${PUBLIC_ACCESS_CIDR}|g" \
    -e "s|__EKS_ACCESS_MODE__|${EKS_ACCESS_MODE}|g" \
    -e "s|__EKS_ENDPOINT_PRIVATE_ACCESS__|${EKS_ENDPOINT_PRIVATE_ACCESS}|g" \
    -e "s|__EKS_ENDPOINT_PUBLIC_ACCESS__|${EKS_ENDPOINT_PUBLIC_ACCESS}|g" \
    docs/eks/byo-network/manifests/cluster.yaml.tpl \
    >"${rendered_cluster}"

log "Cluster manifest:"
cat "${rendered_cluster}"

log "Applying Cluster..."
kubectl apply -f "${rendered_cluster}"

log "Waiting for EKS to become ACTIVE (10-20 min)..."
kubectl -n "${NAMESPACE}" wait --for=condition=Ready --timeout=25m kany8scontrolplane/"${CLUSTER_NAME}" || {
    log "WARN: Kany8sControlPlane not Ready yet. Checking ACK status..."
    kubectl get clusters.eks.services.k8s.aws "${CLUSTER_NAME}" -n "${NAMESPACE}" -o wide 2>&1 || true
    kubectl -n ack-system logs deploy/ack-eks-controller-eks-chart --tail=20 2>&1 || true
}

log "EKS status:"
kubectl get kany8scontrolplane "${CLUSTER_NAME}" -n "${NAMESPACE}" -o wide 2>&1 || true
kubectl get clusters.eks.services.k8s.aws "${CLUSTER_NAME}" -n "${NAMESPACE}" -o wide 2>&1 || true

# ═══════════════════════════════════════════════════════════
# Phase 3: Plugins (kubeconfig-rotator + karpenter-bootstrapper)
# ═══════════════════════════════════════════════════════════
log ""
log "Phase 3: Building and deploying plugins..."

log "Building kubeconfig-rotator..."
make docker-build-eks-plugin EKS_PLUGIN_IMG="${ROTATOR_IMG}"
kind load docker-image "${ROTATOR_IMG}" --name "${KIND_CLUSTER_NAME}"

log "Building karpenter-bootstrapper..."
make docker-build-eks-karpenter-bootstrapper EKS_KARPENTER_BOOTSTRAPPER_IMG="${BOOTSTRAPPER_IMG}"
kind load docker-image "${BOOTSTRAPPER_IMG}" --name "${KIND_CLUSTER_NAME}"

log "Installing Flux..."
FLUX_VERSION="${FLUX_VERSION:-v2.4.0}"
kubectl apply -f "https://github.com/fluxcd/flux2/releases/download/${FLUX_VERSION}/install.yaml"
kubectl -n flux-system rollout status deploy/source-controller --timeout=300s
kubectl -n flux-system rollout status deploy/helm-controller --timeout=300s
log "Flux ready."

log "Deploying plugins..."
# Update image references in kustomization
cd "${repo_root}/config/eks-plugin"
"${repo_root}/bin/kustomize" edit set image "example.com/eks-kubeconfig-rotator=${ROTATOR_IMG}"
cd "${repo_root}/config/eks-karpenter-bootstrapper"
"${repo_root}/bin/kustomize" edit set image "example.com/eks-karpenter-bootstrapper=${BOOTSTRAPPER_IMG}"
cd "${repo_root}"

"${repo_root}/bin/kustomize" build config/overlays/eks-plugin/kind | kubectl apply -f -
"${repo_root}/bin/kustomize" build config/overlays/eks-karpenter-bootstrapper/kind | kubectl apply -f -

kubectl -n ack-system rollout status deploy/eks-kubeconfig-rotator --timeout=300s
kubectl -n ack-system rollout status deploy/eks-karpenter-bootstrapper --timeout=300s
log "Plugins deployed."

# ═══════════════════════════════════════════════════════════
# Phase 4: Enable plugins + wait for Karpenter + nodes
# ═══════════════════════════════════════════════════════════
log ""
log "Phase 4: Enabling plugins and waiting for worker nodes..."

kubectl -n "${NAMESPACE}" annotate cluster "${CLUSTER_NAME}" \
    eks.kany8s.io/kubeconfig-rotator=enabled --overwrite
kubectl -n "${NAMESPACE}" label cluster "${CLUSTER_NAME}" \
    eks.kany8s.io/karpenter=enabled --overwrite

log "Waiting for kubeconfig Secret..."
for i in $(seq 1 120); do
    if kubectl -n "${NAMESPACE}" get secret "${CLUSTER_NAME}-kubeconfig-exec" >/dev/null 2>&1; then
        log "kubeconfig-exec Secret ready."
        break
    fi
    if (( i % 6 == 0 )); then
        log "  Still waiting for kubeconfig Secret... (${i}/120)"
    fi
    sleep 5
done

log "Waiting for FargateProfiles..."
for i in $(seq 1 120); do
    fp_karpenter="$(kubectl -n "${NAMESPACE}" get fargateprofiles.eks.services.k8s.aws "${CLUSTER_NAME}-fargate-karpenter" -o name 2>/dev/null || echo "")"
    fp_coredns="$(kubectl -n "${NAMESPACE}" get fargateprofiles.eks.services.k8s.aws "${CLUSTER_NAME}-fargate-coredns" -o name 2>/dev/null || echo "")"
    if [[ -n "${fp_karpenter}" && -n "${fp_coredns}" ]]; then
        log "FargateProfiles created."
        break
    fi
    if (( i % 6 == 0 )); then
        log "  FargateProfiles: karpenter=${fp_karpenter:-pending} coredns=${fp_coredns:-pending} (${i}/120)"
    fi
    sleep 5
done

log "Waiting for Karpenter HelmRelease..."
for i in $(seq 1 120); do
    if kubectl -n "${NAMESPACE}" get helmreleases.helm.toolkit.fluxcd.io "${CLUSTER_NAME}-karpenter" >/dev/null 2>&1; then
        log "Karpenter HelmRelease created."
        break
    fi
    if (( i % 6 == 0 )); then
        log "  Waiting for HelmRelease... (${i}/120)"
    fi
    sleep 5
done

log "Extracting workload kubeconfig..."
workload_kubeconfig="${ARTIFACTS_DIR}/workload-kubeconfig"
for i in $(seq 1 30); do
    if kubectl -n "${NAMESPACE}" get secret "${CLUSTER_NAME}-kubeconfig-exec" >/dev/null 2>&1; then
        kubectl -n "${NAMESPACE}" get secret "${CLUSTER_NAME}-kubeconfig-exec" \
            -o jsonpath='{.data.value}' | base64 -d > "${workload_kubeconfig}"
        chmod 0600 "${workload_kubeconfig}"
        log "Workload kubeconfig saved: ${workload_kubeconfig}"
        break
    fi
    sleep 10
done

log "Waiting for at least one worker node (Karpenter → EC2)..."
for i in $(seq 1 240); do
    if [[ -f "${workload_kubeconfig}" ]]; then
        node_count="$(KUBECONFIG="${workload_kubeconfig}" kubectl get nodes --no-headers 2>/dev/null | wc -l || echo 0)"
        if (( node_count > 0 )); then
            log "Worker node(s) joined! (${node_count} nodes)"
            KUBECONFIG="${workload_kubeconfig}" kubectl get nodes -o wide 2>&1 || true
            break
        fi
    fi
    if (( i % 12 == 0 )); then
        log "  Waiting for nodes... (${i}/240, ~$((i*5/60))min elapsed)"
        # Show bootstrapper logs for debugging
        kubectl -n ack-system logs deploy/eks-karpenter-bootstrapper --tail=5 2>&1 || true
    fi
    sleep 5
done

# ═══════════════════════════════════════════════════════════
# Done
# ═══════════════════════════════════════════════════════════
log ""
log "=========================================="
log "  EKS Full-Stack Setup Complete!"
log "=========================================="
log ""
log "  CLUSTER_NAME:     ${CLUSTER_NAME}"
log "  AWS_REGION:       ${AWS_REGION}"
log "  EKS Version:      ${EKS_VERSION}"
log "  Workload config:  ${workload_kubeconfig}"
log "  Artifacts:        ${ARTIFACTS_DIR}"
log "  Log:              ${log_file}"
log ""
log "Connect to workload cluster:"
log "  export KUBECONFIG=${workload_kubeconfig}"
log "  kubectl get nodes"
log "  kubectl get pods -A"
log ""
log "Cleanup (IMPORTANT - EKS + NAT costs money!):"
log "  kubectl delete cluster.cluster.x-k8s.io ${CLUSTER_NAME} -n ${NAMESPACE}"
log "  # Wait for EKS + IAM + Fargate deletion..."
log "  kubectl delete -f ${rendered_network}"
log "  # Wait for VPC + NAT + IGW deletion..."
