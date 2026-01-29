#!/usr/bin/env bash
set -euo pipefail

timestamp="$(date +%Y%m%d%H%M%S)"

KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-kany8s-acceptance-self-managed-${timestamp}}"
KUBECTL_CONTEXT="${KUBECTL_CONTEXT:-kind-${KIND_CLUSTER_NAME}}"

IMG="${IMG:-example.com/kany8s:acceptance-self-managed}"
CLUSTERCTL_VERSION="${CLUSTERCTL_VERSION:-v1.12.2}"

NAMESPACE="${NAMESPACE:-default}"
CLUSTER_NAME="${CLUSTER_NAME:-demo-self-managed-docker}"
CLUSTER_WAIT_TIMEOUT="${CLUSTER_WAIT_TIMEOUT:-30m}"

CLEANUP="${CLEANUP:-true}"

ARTIFACTS_DIR="${ARTIFACTS_DIR:-/tmp/kany8s-acceptance-self-managed-${timestamp}}"
KUBECONFIG_FILE="${KUBECONFIG_FILE:-${ARTIFACTS_DIR}/kubeconfig}"
WORKLOAD_KUBECONFIG_FILE="${WORKLOAD_KUBECONFIG_FILE:-${ARTIFACTS_DIR}/workload.kubeconfig}"

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "${repo_root}"

mkdir -p "${ARTIFACTS_DIR}"
log_file="${ARTIFACTS_DIR}/acceptance-self-managed.log"
exec > >(tee -a "${log_file}") 2>&1

export KUBECONFIG="${KUBECONFIG_FILE}"

kustomization_path="${repo_root}/config/manager/kustomization.yaml"
kustomization_backup=""

need_cmd() {
	local cmd
	cmd="$1"
	command -v "${cmd}" >/dev/null 2>&1 || {
		echo "error: required command not found: ${cmd}" >&2
		exit 1
	}
}

k() {
	kubectl --context "${KUBECTL_CONTEXT}" "$@"
}

backup_kustomization() {
	if [[ -f "${kustomization_path}" ]]; then
		kustomization_backup="${ARTIFACTS_DIR}/kustomization.yaml.bak"
		cp "${kustomization_path}" "${kustomization_backup}"
	fi
}

restore_kustomization() {
	if [[ -n "${kustomization_backup}" && -f "${kustomization_backup}" ]]; then
		cp "${kustomization_backup}" "${kustomization_path}"
	fi
}

collect_diagnostics() {
	echo "==> Collecting diagnostics into ${ARTIFACTS_DIR}"

	local diag_dir
	diag_dir="${ARTIFACTS_DIR}/diagnostics"
	mkdir -p "${diag_dir}"

	{
		echo "kind clusters:";
		kind get clusters || true
	} >"${diag_dir}/kind.txt" 2>&1 || true

	if [[ -f "${KUBECONFIG_FILE}" ]]; then
		kubectl --kubeconfig "${KUBECONFIG_FILE}" config get-contexts >"${diag_dir}/kubeconfig-contexts.txt" 2>&1 || true
		kubectl --kubeconfig "${KUBECONFIG_FILE}" config view --minify >"${diag_dir}/kubeconfig-minify.yaml" 2>&1 || true
	fi

	k get nodes -o wide >"${diag_dir}/mgmt-nodes.txt" 2>&1 || true
	k get deployments -A -o wide >"${diag_dir}/deployments.txt" 2>&1 || true
	k get events -A --sort-by=.metadata.creationTimestamp >"${diag_dir}/events.txt" 2>&1 || true

	k -n "${NAMESPACE}" get cluster "${CLUSTER_NAME}" -o yaml >"${diag_dir}/cluster.yaml" 2>&1 || true
	k -n "${NAMESPACE}" get kany8skubeadmcontrolplane "${CLUSTER_NAME}" -o yaml >"${diag_dir}/kany8skubeadmcontrolplane.yaml" 2>&1 || true

	k -n capi-system logs deploy/capi-controller-manager --tail=200 >"${diag_dir}/capi-controller-logs.txt" 2>&1 || true
	k -n capd-system logs deploy/capd-controller-manager --tail=200 >"${diag_dir}/capd-controller-logs.txt" 2>&1 || true
	k -n cabpk-system logs deploy/cabpk-controller-manager --tail=200 >"${diag_dir}/cabpk-controller-logs.txt" 2>&1 || true
	k -n kany8s-system logs deploy/kany8s-controller-manager -c manager --tail=200 >"${diag_dir}/kany8s-controller-logs.txt" 2>&1 || true
}

cleanup() {
	restore_kustomization

	if [[ "${CLEANUP}" == "true" ]]; then
		echo "==> Cleaning up kind cluster ${KIND_CLUSTER_NAME}"
		kind delete cluster --name "${KIND_CLUSTER_NAME}" --kubeconfig "${KUBECONFIG_FILE}" || true
	else
		echo "==> CLEANUP=false; keeping kind cluster ${KIND_CLUSTER_NAME}"
		echo "    KUBECONFIG=${KUBECONFIG_FILE}"
		echo "    kubectl --context ${KUBECTL_CONTEXT} get nodes"
		echo "    clusterctl --kubeconfig ${KUBECONFIG_FILE} describe cluster -n ${NAMESPACE} ${CLUSTER_NAME}"
	fi
}

on_exit() {
	local rc
	rc="$?"
	if [[ "${rc}" -ne 0 ]]; then
		collect_diagnostics
	fi
	cleanup
	exit "${rc}"
}
trap on_exit EXIT

need_cmd docker
need_cmd kind
need_cmd kubectl
need_cmd make
need_cmd go
need_cmd curl

echo "==> Creating kind management cluster ${KIND_CLUSTER_NAME}"
kind create cluster --name "${KIND_CLUSTER_NAME}" --wait 60s --kubeconfig "${KUBECONFIG_FILE}"

echo "==> Verifying management cluster is reachable"
k get nodes -o wide

echo "==> Building controller image ${IMG}"
make docker-build IMG="${IMG}"

echo "==> Loading controller image into kind"
kind load docker-image "${IMG}" --name "${KIND_CLUSTER_NAME}"

echo "==> Building clusterctl components bundle for Kany8s"
backup_kustomization
make build-installer IMG="${IMG}"

bundle_path="$(realpath "${repo_root}/dist/install.yaml")"
clusterctl_config="${ARTIFACTS_DIR}/clusterctl.yaml"
cat >"${clusterctl_config}" <<EOF
providers:
  - name: kany8s
    type: ControlPlaneProvider
    url: file://${bundle_path}
EOF

clusterctl_bin="${ARTIFACTS_DIR}/clusterctl-${CLUSTERCTL_VERSION}"
if [[ ! -x "${clusterctl_bin}" ]]; then
	curl -fsSL -o "${clusterctl_bin}" "https://github.com/kubernetes-sigs/cluster-api/releases/download/${CLUSTERCTL_VERSION}/clusterctl-linux-amd64"
	chmod +x "${clusterctl_bin}"
fi

"${clusterctl_bin}" version

echo "==> Installing Cluster API providers (CAPD + CABPK + Kany8s)"
if ! k get namespace kany8s-system >/dev/null 2>&1; then
	k create namespace kany8s-system
fi

echo "==> clusterctl init --infrastructure docker --bootstrap kubeadm --control-plane kany8s"
"${clusterctl_bin}" --config "${clusterctl_config}" init \
	--infrastructure docker \
	--bootstrap kubeadm \
	--control-plane kany8s \
	--wait-providers \
	--kubeconfig-context "${KUBECTL_CONTEXT}"

echo "==> Applying self-managed sample manifests"
k apply -f "${repo_root}/examples/self-managed-docker/cluster.yaml"

echo "==> Waiting for Cluster RemoteConnectionProbe=True"
k -n "${NAMESPACE}" wait --for=condition=RemoteConnectionProbe --timeout="${CLUSTER_WAIT_TIMEOUT}" "cluster/${CLUSTER_NAME}"

echo "==> Waiting for Cluster Available=True"
k -n "${NAMESPACE}" wait --for=condition=Available --timeout="${CLUSTER_WAIT_TIMEOUT}" "cluster/${CLUSTER_NAME}"

echo "==> Fetching workload kubeconfig"
echo "==> clusterctl get kubeconfig -n ${NAMESPACE} ${CLUSTER_NAME}"
"${clusterctl_bin}" --config "${clusterctl_config}" get kubeconfig -n "${NAMESPACE}" "${CLUSTER_NAME}" >"${WORKLOAD_KUBECONFIG_FILE}"

echo "==> Verifying workload cluster connectivity"
kubectl --kubeconfig "${WORKLOAD_KUBECONFIG_FILE}" get nodes -o wide

echo "==> OK: self-managed acceptance test passed"
