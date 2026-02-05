#!/usr/bin/env bash
set -euo pipefail

timestamp="${TIMESTAMP:-$(date +%Y%m%d%H%M%S)}"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-kany8s-acc-infra-identity-${timestamp}}"
ARTIFACTS_DIR="${ARTIFACTS_DIR:-/tmp/kany8s-acceptance-kro-infra-cluster-identity-${timestamp}}"
CLEANUP="${CLEANUP:-true}"

command -v kind >/dev/null 2>&1 || { echo "error: kind not found" >&2; exit 1; }

echo "==> Pre-clean kind cluster: ${KIND_CLUSTER_NAME}"
kind delete cluster --name "${KIND_CLUSTER_NAME}" >/dev/null 2>&1 || true

echo "==> Running acceptance (kro infra cluster identity)"
echo "    ARTIFACTS_DIR=${ARTIFACTS_DIR}"
echo "    KIND_CLUSTER_NAME=${KIND_CLUSTER_NAME}"
echo "    CLEANUP=${CLEANUP}"

KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME}" \
ARTIFACTS_DIR="${ARTIFACTS_DIR}" \
CLEANUP="${CLEANUP}" \
bash "${repo_root}/hack/acceptance-test-kro-infra-cluster-identity.sh"
