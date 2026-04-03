#!/usr/bin/env bash
# setup-eks-management.sh — Kind 管理クラスタに EKS 作成用の土台をセットアップ
#
# Usage:
#   bash hack/setup-eks-management.sh
#
# 環境変数:
#   AWS_REGION              (default: ap-northeast-1)
#   KIND_CLUSTER_NAME       (default: kany8s-eks)
#   CLEANUP                 (default: false) — true にすると既存 Kind クラスタを再作成
#   WITH_CAPI               (default: true)  — CAPI core もインストール
#   KRO_VERSION             (default: 0.7.1)
#   CAPI_VERSION            (default: v1.12.2)
#   IMG                     (default: example.com/kany8s:eks-smoke)
#
# 前提:
#   - devbox shell 内で実行（go, kind, kubectl, helm, aws, clusterctl が PATH に必要）
#   - aws sts get-caller-identity が通ること
#
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

# ─── 設定 ───────────────────────────────────────────────
AWS_REGION="${AWS_REGION:-ap-northeast-1}"
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-kany8s-eks}"
CLEANUP="${CLEANUP:-false}"
WITH_CAPI="${WITH_CAPI:-true}"
KRO_VERSION="${KRO_VERSION:-0.7.1}"
CAPI_VERSION="${CAPI_VERSION:-v1.12.2}"
CERT_MANAGER_VERSION="${CERT_MANAGER_VERSION:-v1.16.3}"
IMG="${IMG:-example.com/kany8s:eks-smoke}"
ACK_SYSTEM_NAMESPACE="ack-system"

ARTIFACTS_DIR="${ARTIFACTS_DIR:-/tmp/kany8s-eks-$(date +%Y%m%d%H%M%S)}"
mkdir -p "${ARTIFACTS_DIR}"
log_file="${ARTIFACTS_DIR}/setup.log"
exec > >(tee -a "${log_file}") 2>&1

# ─── ユーティリティ ──────────────────────────────────────
log() { echo "[setup] $(date +%H:%M:%S) $*"; }
need_cmd() {
    command -v "$1" >/dev/null 2>&1 || {
        echo "ERROR: required command not found: $1" >&2
        exit 1
    }
}

# Workaround: devbox/nix environments can exceed Linux's ARG_MAX limit,
# causing "Argument list too long" errors for some binaries.
# This wrapper runs kubectl with a minimal environment.
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

# ─── Step 0: 前提チェック ────────────────────────────────
log "Checking prerequisites..."
for cmd in docker kind kubectl helm aws jq curl make go; do
    need_cmd "${cmd}"
done

log "Verifying AWS credentials..."
aws sts get-caller-identity --output table
log "AWS_REGION=${AWS_REGION}"

# ─── Step 1: Kind 管理クラスタ ───────────────────────────
if kind get clusters 2>/dev/null | grep -q "^${KIND_CLUSTER_NAME}$"; then
    if [[ "${CLEANUP}" == "true" ]]; then
        log "Deleting existing Kind cluster: ${KIND_CLUSTER_NAME}"
        kind delete cluster --name "${KIND_CLUSTER_NAME}"
    else
        log "Kind cluster '${KIND_CLUSTER_NAME}' already exists. Reusing."
    fi
fi

if ! kind get clusters 2>/dev/null | grep -q "^${KIND_CLUSTER_NAME}$"; then
    log "Creating Kind cluster: ${KIND_CLUSTER_NAME}"
    kind create cluster --name "${KIND_CLUSTER_NAME}" --wait 60s
fi

kubectl config use-context "kind-${KIND_CLUSTER_NAME}"
kubectl get nodes -o wide
log "Kind cluster ready."

# ─── Step 2: CAPI / cert-manager ────────────────────────
if [[ "${WITH_CAPI}" == "true" ]]; then
    log "Installing CAPI core (${CAPI_VERSION}) + cert-manager..."
    export CLUSTER_TOPOLOGY=true

    # clusterctl init は cert-manager も自動インストール
    clusterctl init \
        --core "cluster-api:${CAPI_VERSION}" \
        --bootstrap "kubeadm:${CAPI_VERSION}" \
        --control-plane "kubeadm:${CAPI_VERSION}" \
        --infrastructure "docker:${CAPI_VERSION}" \
        --wait-providers || {
            log "WARN: clusterctl init failed. Trying with cert-manager only..."
            kubectl apply -f "https://github.com/cert-manager/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml"
            kubectl -n cert-manager rollout status deploy/cert-manager --timeout=300s
            kubectl -n cert-manager rollout status deploy/cert-manager-webhook --timeout=300s
            kubectl -n cert-manager rollout status deploy/cert-manager-cainjector --timeout=300s
        }

    kubectl get deployments -n capi-system 2>/dev/null || true
    log "CAPI / cert-manager ready."
else
    log "Installing cert-manager only (${CERT_MANAGER_VERSION})..."
    kubectl apply -f "https://github.com/cert-manager/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml"
    kubectl -n cert-manager rollout status deploy/cert-manager --timeout=300s
    kubectl -n cert-manager rollout status deploy/cert-manager-webhook --timeout=300s
    kubectl -n cert-manager rollout status deploy/cert-manager-cainjector --timeout=300s
    log "cert-manager ready."
fi

# ─── Step 3: kro ────────────────────────────────────────
log "Installing kro v${KRO_VERSION}..."
kubectl create namespace kro-system 2>/dev/null || true
curl -fsSL -o /tmp/kro-core-install-manifests.yaml \
    "https://github.com/kubernetes-sigs/kro/releases/download/v${KRO_VERSION}/kro-core-install-manifests.yaml"
kubectl apply -f /tmp/kro-core-install-manifests.yaml
kubectl -n kro-system rollout status deploy/kro --timeout=180s

# kro v0.7.1 workaround: 広い RBAC
kubectl apply -f "${repo_root}/test/acceptance_test/manifests/kro/rbac-unrestricted.yaml"
log "kro ready."

# ─── Step 4: AWS ACK (iam + ec2 + eks) ──────────────────
log "Installing AWS ACK controllers..."
kubectl create namespace "${ACK_SYSTEM_NAMESPACE}" 2>/dev/null || true

# AWS credentials Secret
tmp_creds=""
if [[ -f "$HOME/.aws/credentials" ]]; then
    tmp_creds="$HOME/.aws/credentials"
else
    if [[ -z "${AWS_ACCESS_KEY_ID:-}" || -z "${AWS_SECRET_ACCESS_KEY:-}" ]]; then
        log "ERROR: ~/.aws/credentials not found and AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY not set"
        exit 1
    fi
    tmp_creds="$(mktemp)"
    cat >"${tmp_creds}" <<EOF
[default]
aws_access_key_id = ${AWS_ACCESS_KEY_ID}
aws_secret_access_key = ${AWS_SECRET_ACCESS_KEY}
EOF
    if [[ -n "${AWS_SESSION_TOKEN:-}" ]]; then
        echo "aws_session_token = ${AWS_SESSION_TOKEN}" >>"${tmp_creds}"
    fi
fi

kubectl -n "${ACK_SYSTEM_NAMESPACE}" create secret generic aws-creds \
    --from-file=credentials="${tmp_creds}" \
    --dry-run=client -o yaml | kubectl apply -f -

if [[ "${tmp_creds}" == /tmp/* ]]; then
    rm -f "${tmp_creds}"
fi

log "Logging in to ECR public..."
aws ecr-public get-login-password --region us-east-1 | \
    helm registry login --username AWS --password-stdin public.ecr.aws

log "Fetching latest ACK controller versions..."
IAM_RELEASE_VERSION="$(curl -sL https://api.github.com/repos/aws-controllers-k8s/iam-controller/releases/latest | jq -r '.tag_name | ltrimstr("v")')"
EC2_RELEASE_VERSION="$(curl -sL https://api.github.com/repos/aws-controllers-k8s/ec2-controller/releases/latest | jq -r '.tag_name | ltrimstr("v")')"
EKS_RELEASE_VERSION="$(curl -sL https://api.github.com/repos/aws-controllers-k8s/eks-controller/releases/latest | jq -r '.tag_name | ltrimstr("v")')"
log "ACK versions: iam=${IAM_RELEASE_VERSION} ec2=${EC2_RELEASE_VERSION} eks=${EKS_RELEASE_VERSION}"

log "Installing ACK IAM controller..."
helm upgrade --install --create-namespace -n "${ACK_SYSTEM_NAMESPACE}" ack-iam-controller \
    oci://public.ecr.aws/aws-controllers-k8s/iam-chart \
    --version="${IAM_RELEASE_VERSION}" \
    --set=aws.region="${AWS_REGION}" \
    --set=aws.credentials.secretName=aws-creds

log "Installing ACK EC2 controller..."
helm upgrade --install --create-namespace -n "${ACK_SYSTEM_NAMESPACE}" ack-ec2-controller \
    oci://public.ecr.aws/aws-controllers-k8s/ec2-chart \
    --version="${EC2_RELEASE_VERSION}" \
    --set=aws.region="${AWS_REGION}" \
    --set=aws.credentials.secretName=aws-creds

log "Installing ACK EKS controller..."
helm upgrade --install --create-namespace -n "${ACK_SYSTEM_NAMESPACE}" ack-eks-controller \
    oci://public.ecr.aws/aws-controllers-k8s/eks-chart \
    --version="${EKS_RELEASE_VERSION}" \
    --set=aws.region="${AWS_REGION}" \
    --set=aws.credentials.secretName=aws-creds

log "Waiting for ACK controllers to be ready..."
kubectl -n "${ACK_SYSTEM_NAMESPACE}" rollout status deploy/ack-iam-controller-iam-chart --timeout=180s
kubectl -n "${ACK_SYSTEM_NAMESPACE}" rollout status deploy/ack-ec2-controller-ec2-chart --timeout=180s
kubectl -n "${ACK_SYSTEM_NAMESPACE}" rollout status deploy/ack-eks-controller-eks-chart --timeout=180s
log "ACK controllers ready."

# ─── Step 5: Kany8s deploy ──────────────────────────────
log "Building and deploying Kany8s..."

# Workaround: make install/deploy calls kubectl internally.
# Create a wrapper script that passes a clean env to avoid ARG_MAX issues.
_kubectl_wrapper="$(mktemp)"
cat >"${_kubectl_wrapper}" <<'WRAPPER'
#!/usr/bin/env bash
exec env -i HOME="${HOME}" PATH="${PATH}" KUBECONFIG="${KUBECONFIG:-}" USER="${USER:-}" TERM="${TERM:-dumb}" \
    "$(dirname "$0")/../.devbox/nix/profile/default/bin/kubectl" "$@" 2>/dev/null \
    || command kubectl "$@"
WRAPPER
# Use a stable wrapper path that make can find
_kubectl_bin="${repo_root}/bin/kubectl-wrapper"
cp "${_kubectl_wrapper}" "${_kubectl_bin}"
chmod +x "${_kubectl_bin}"
rm -f "${_kubectl_wrapper}"

make manifests
log "Installing CRDs..."
"${repo_root}/bin/kustomize" build config/crd | kubectl apply -f -

log "Building Docker image: ${IMG}"
make docker-build IMG="${IMG}"

log "Loading image into Kind..."
kind load docker-image "${IMG}" --name "${KIND_CLUSTER_NAME}"

log "Deploying Kany8s controller..."
cd "${repo_root}/config/manager" && "${repo_root}/bin/kustomize" edit set image controller="${IMG}" && cd "${repo_root}"
"${repo_root}/bin/kustomize" build config/default | kubectl apply -f -

kubectl -n kany8s-system rollout status deployment/kany8s-controller-manager --timeout=180s
log "Kany8s controller ready."

# ─── 完了 ───────────────────────────────────────────────
log ""
log "=========================================="
log "  EKS management cluster setup complete!"
log "=========================================="
log ""
log "  Kind cluster:  ${KIND_CLUSTER_NAME}"
log "  Context:       kind-${KIND_CLUSTER_NAME}"
log "  AWS region:    ${AWS_REGION}"
log "  Artifacts:     ${ARTIFACTS_DIR}"
log "  Log:           ${log_file}"
log ""
log "Components installed:"
log "  - Kind management cluster"
if [[ "${WITH_CAPI}" == "true" ]]; then
log "  - CAPI core (${CAPI_VERSION})"
fi
log "  - cert-manager (${CERT_MANAGER_VERSION})"
log "  - kro (${KRO_VERSION})"
log "  - ACK IAM controller (${IAM_RELEASE_VERSION})"
log "  - ACK EC2 controller (${EC2_RELEASE_VERSION})"
log "  - ACK EKS controller (${EKS_RELEASE_VERSION})"
log "  - Kany8s controller (${IMG})"
log ""
log "Next: Apply EKS RGD and create a cluster."
log "  See: docs/eks/README.md (Step 7-8)"
