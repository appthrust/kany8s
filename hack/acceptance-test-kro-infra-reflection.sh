#!/usr/bin/env bash
set -euo pipefail

timestamp="$(date +%Y%m%d%H%M%S)"

KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-kany8s-acceptance-infra-${timestamp}}"
KUBECTL_CONTEXT="${KUBECTL_CONTEXT:-kind-${KIND_CLUSTER_NAME}}"

NAMESPACE="${NAMESPACE:-default}"
CLUSTER_NAME="${CLUSTER_NAME:-demo-cluster}"
KRO_VERSION="${KRO_VERSION:-0.7.1}"
IMG="${IMG:-example.com/kany8s:acceptance-kro-infra}"
CLEANUP="${CLEANUP:-true}"

ARTIFACTS_DIR="${ARTIFACTS_DIR:-/tmp/kany8s-acceptance-kro-infra-${timestamp}}"
KUBECONFIG_FILE="${KUBECONFIG_FILE:-${ARTIFACTS_DIR}/kubeconfig}"

RGD_NAME="demo-infra.kro.run"
RGD_INSTANCE_CRD="demoinfrastructures.kro.run"

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "${repo_root}"

KRO_RBAC_WORKAROUND_MANIFEST="${KRO_RBAC_WORKAROUND_MANIFEST:-test/acceptance_test/manifests/kro/rbac-unrestricted.yaml}"
KRO_RGD_MANIFEST="${KRO_RGD_MANIFEST:-test/acceptance_test/manifests/kro/infra/rgd.yaml}"
KANY8S_CLUSTER_TEMPLATE="${KANY8S_CLUSTER_TEMPLATE:-test/acceptance_test/manifests/kro/kany8scluster.yaml.tpl}"

mkdir -p "${ARTIFACTS_DIR}"

export KUBECONFIG="${KUBECONFIG_FILE}"

log_file="${ARTIFACTS_DIR}/acceptance-infra.log"
touch "${log_file}"
exec > >(tee -a "${log_file}") 2>&1

echo "error: kro infra reflection acceptance script is not implemented yet" >&2
echo "see docs/issues/kany8cluster-at-todo.md" >&2
exit 1
