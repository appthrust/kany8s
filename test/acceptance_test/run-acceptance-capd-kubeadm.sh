#!/usr/bin/env bash
set -euo pipefail

timestamp="${TIMESTAMP:-$(date +%Y%m%d%H%M%S)}"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-kany8s-capd-${timestamp}}"
ARTIFACTS_DIR="${ARTIFACTS_DIR:-/tmp/kany8s-acceptance-capd-kubeadm-${timestamp}}"
CLEANUP="${CLEANUP:-true}"

NAMESPACE="${NAMESPACE:-default}"
CLUSTER_NAME="${CLUSTER_NAME:-demo-self-managed-docker}"

# If true, we won't delete leftover workload containers before running.
REUSE_EXISTING_WORKLOAD_DOCKER_CONTAINERS="${REUSE_EXISTING_WORKLOAD_DOCKER_CONTAINERS:-false}"

command -v kind >/dev/null 2>&1 || { echo "error: kind not found" >&2; exit 1; }
command -v docker >/dev/null 2>&1 || { echo "error: docker not found" >&2; exit 1; }

echo "==> Pre-clean kind cluster: ${KIND_CLUSTER_NAME}"
kind delete cluster --name "${KIND_CLUSTER_NAME}" >/dev/null 2>&1 || true

if [[ "${REUSE_EXISTING_WORKLOAD_DOCKER_CONTAINERS}" != "true" ]]; then
	# Workload cluster containers are created on the host Docker daemon via CAPD.
	# They are NOT removed by deleting the kind management cluster.
	echo "==> Pre-clean workload Docker containers for CLUSTER_NAME=${CLUSTER_NAME}"
	docker ps -a --format '{{.Names}}' | while read -r name; do
		[[ -n "${name}" ]] || continue
		if [[ "${name}" == "${CLUSTER_NAME}-"* ]]; then
			docker rm -f "${name}" >/dev/null 2>&1 || true
		fi
	done
fi

echo "==> Running acceptance (CAPD + kubeadm)"
echo "    ARTIFACTS_DIR=${ARTIFACTS_DIR}"
echo "    KIND_CLUSTER_NAME=${KIND_CLUSTER_NAME}"
echo "    NAMESPACE=${NAMESPACE}"
echo "    CLUSTER_NAME=${CLUSTER_NAME}"
echo "    CLEANUP=${CLEANUP}"

KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME}" \
ARTIFACTS_DIR="${ARTIFACTS_DIR}" \
CLEANUP="${CLEANUP}" \
NAMESPACE="${NAMESPACE}" \
CLUSTER_NAME="${CLUSTER_NAME}" \
REUSE_EXISTING_WORKLOAD_DOCKER_CONTAINERS="${REUSE_EXISTING_WORKLOAD_DOCKER_CONTAINERS}" \
bash "${repo_root}/hack/acceptance-test-capd-kubeadm.sh"
