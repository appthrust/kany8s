#!/usr/bin/env bash
# create-eks-cluster.sh — EKS smoke test クラスタを作成
#
# Usage: bash hack/create-eks-cluster.sh
#
# 前提: setup-eks-management.sh が完了していること
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

# kubectl ARG_MAX workaround
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

log() { echo "[eks] $(date +%H:%M:%S) $*"; }

# ─── 設定 ───────────────────────────────────────────────
AWS_REGION="${AWS_REGION:-ap-northeast-1}"
CLUSTER_NAME="${CLUSTER_NAME:-demo-eks-135-$(date +%Y%m%d%H%M%S)}"
NAMESPACE="${NAMESPACE:-default}"
KUBERNETES_VERSION="${KUBERNETES_VERSION:-1.35}"
VPC_CIDR="${VPC_CIDR:-10.35.0.0/16}"
SUBNET_A_CIDR="${SUBNET_A_CIDR:-10.35.0.0/24}"
SUBNET_A_AZ="${SUBNET_A_AZ:-ap-northeast-1a}"
SUBNET_B_CIDR="${SUBNET_B_CIDR:-10.35.1.0/24}"
SUBNET_B_AZ="${SUBNET_B_AZ:-ap-northeast-1c}"

# Public IP を自動取得（指定がなければ）
if [[ -z "${PUBLIC_ACCESS_CIDR:-}" ]]; then
    my_ip="$(curl -fsSL https://checkip.amazonaws.com | tr -d '\n')"
    PUBLIC_ACCESS_CIDR="${my_ip}/32"
fi

log "CLUSTER_NAME=${CLUSTER_NAME}"
log "AWS_REGION=${AWS_REGION}"
log "PUBLIC_ACCESS_CIDR=${PUBLIC_ACCESS_CIDR}"

kubectl config use-context kind-kany8s-eks

# ─── Step 7: Apply EKS RGD ──────────────────────────────
log "Applying EKS Control Plane smoke RGD..."
kubectl apply -f docs/eks/manifests/eks-control-plane-smoke-rgd.yaml
kubectl wait --for=condition=ResourceGraphAccepted --timeout=120s rgd/eks-control-plane-smoke.kro.run
log "RGD accepted!"

# ─── Step 8: Render & Apply Cluster ─────────────────────
rendered="/tmp/eks-cluster-${CLUSTER_NAME}.yaml"
sed \
    -e "s|__CLUSTER_NAME__|${CLUSTER_NAME}|g" \
    -e "s|__NAMESPACE__|${NAMESPACE}|g" \
    -e "s|__KUBERNETES_VERSION__|${KUBERNETES_VERSION}|g" \
    -e "s|__AWS_REGION__|${AWS_REGION}|g" \
    -e "s|__VPC_CIDR__|${VPC_CIDR}|g" \
    -e "s|__SUBNET_A_CIDR__|${SUBNET_A_CIDR}|g" \
    -e "s|__SUBNET_A_AZ__|${SUBNET_A_AZ}|g" \
    -e "s|__SUBNET_B_CIDR__|${SUBNET_B_CIDR}|g" \
    -e "s|__SUBNET_B_AZ__|${SUBNET_B_AZ}|g" \
    docs/eks/manifests/cluster.yaml.tpl \
| sed \
    -e 's|^    # publicAccessCIDRs:|    publicAccessCIDRs:|' \
    -e "s|^    #   - \"203.0.113.10/32\"|      - \"${PUBLIC_ACCESS_CIDR}\"|" \
>"${rendered}"

log "Rendered manifest: ${rendered}"
cat "${rendered}"
echo ""

log "Applying Cluster + Kany8sCluster + Kany8sControlPlane..."
kubectl apply -f "${rendered}"

log ""
log "=========================================="
log "  EKS cluster creation started!"
log "=========================================="
log ""
log "  CLUSTER_NAME:  ${CLUSTER_NAME}"
log "  AWS_REGION:    ${AWS_REGION}"
log "  EKS Version:   ${KUBERNETES_VERSION}"
log "  Public CIDR:   ${PUBLIC_ACCESS_CIDR}"
log "  Manifest:      ${rendered}"
log ""
log "EKS creation takes 10-20 minutes."
log ""
log "Monitor:"
log "  kubectl get kany8scontrolplane ${CLUSTER_NAME} -o wide"
log "  kubectl get clusters.eks.services.k8s.aws ${CLUSTER_NAME} -o wide"
log "  kubectl -n ack-system logs deploy/ack-eks-controller-eks-chart --tail=50"
log ""
log "Wait for Ready:"
log "  kubectl wait --for=condition=Ready --timeout=25m kany8scontrolplane/${CLUSTER_NAME}"
log ""
log "Cleanup (IMPORTANT - EKS costs money!):"
log "  kubectl delete cluster.cluster.x-k8s.io ${CLUSTER_NAME}"
log "  kubectl delete kany8scontrolplane ${CLUSTER_NAME}"
