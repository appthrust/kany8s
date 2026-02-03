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
KRO_CORE_INSTALL_MANIFEST="${KRO_CORE_INSTALL_MANIFEST:-test/acceptance_test/vendor/kro/v${KRO_VERSION}/kro-core-install-manifests.yaml}"
KRO_RGD_MANIFEST="${KRO_RGD_MANIFEST:-test/acceptance_test/manifests/kro/infra/rgd.yaml}"
KANY8S_CLUSTER_TEMPLATE="${KANY8S_CLUSTER_TEMPLATE:-test/acceptance_test/manifests/kro/kany8scluster.yaml.tpl}"

mkdir -p "${ARTIFACTS_DIR}"

export KUBECONFIG="${KUBECONFIG_FILE}"

log_file="${ARTIFACTS_DIR}/acceptance-infra.log"
touch "${log_file}"
exec > >(tee -a "${log_file}") 2>&1

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

cleanup() {
	restore_kustomization

	if [[ "${CLEANUP}" == "true" ]]; then
		echo "==> Cleaning up kind cluster ${KIND_CLUSTER_NAME}"
		kind delete cluster --name "${KIND_CLUSTER_NAME}" --kubeconfig "${KUBECONFIG_FILE}" || true
	else
		echo "==> CLEANUP=false; keeping kind cluster ${KIND_CLUSTER_NAME}"
		echo "==> kubectl context: ${KUBECTL_CONTEXT}"
		echo "==> kubeconfig: ${KUBECONFIG_FILE}"
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

	k get nodes -o wide >"${diag_dir}/nodes.txt" 2>&1 || true
	k get events -A --sort-by=.metadata.creationTimestamp >"${diag_dir}/events.txt" 2>&1 || true

	k -n kro-system get all -o wide >"${diag_dir}/kro-system.txt" 2>&1 || true
	k -n kro-system logs deploy/kro --tail=200 >"${diag_dir}/kro-logs.txt" 2>&1 || true
	k get rgd "${RGD_NAME}" -o yaml >"${diag_dir}/rgd.yaml" 2>&1 || true
	k get crd "${RGD_INSTANCE_CRD}" -o yaml >"${diag_dir}/rgd-instance-crd.yaml" 2>&1 || true

	k -n kany8s-system get all -o wide >"${diag_dir}/kany8s-system.txt" 2>&1 || true
	k -n kany8s-system logs deploy/kany8s-controller-manager -c manager --tail=200 >"${diag_dir}/kany8s-controller-logs.txt" 2>&1 || true

	k -n "${NAMESPACE}" get kany8scluster "${CLUSTER_NAME}" -o yaml >"${diag_dir}/kany8scluster.yaml" 2>&1 || true
	k -n "${NAMESPACE}" get "${RGD_INSTANCE_CRD}" "${CLUSTER_NAME}" -o yaml >"${diag_dir}/rgd-instance.yaml" 2>&1 || true
}

on_exit() {
	local rc
	rc=$?

	if [[ "${rc}" -ne 0 ]]; then
		collect_diagnostics || true
	fi

	cleanup || true
}

trap on_exit EXIT

need_cmd docker
need_cmd kind
need_cmd kubectl
need_cmd make
need_cmd go
need_cmd curl

echo "==> Creating kind cluster ${KIND_CLUSTER_NAME}"
kind create cluster --name "${KIND_CLUSTER_NAME}" --wait 60s --kubeconfig "${KUBECONFIG_FILE}"

k get nodes -o wide

echo "==> Ensuring kro-system namespace exists"
k get namespace kro-system >/dev/null 2>&1 || k create namespace kro-system

mkdir -p "$(dirname "${KRO_CORE_INSTALL_MANIFEST}")"

if [[ ! -f "${KRO_CORE_INSTALL_MANIFEST}" ]]; then
	echo "==> Downloading kro install manifest to ${KRO_CORE_INSTALL_MANIFEST}"
	curl -fsSL -o "${KRO_CORE_INSTALL_MANIFEST}" \
		"https://github.com/kubernetes-sigs/kro/releases/download/v${KRO_VERSION}/kro-core-install-manifests.yaml"
fi

k apply -f "${KRO_CORE_INSTALL_MANIFEST}"

echo "==> Waiting for kro rollout"
k -n kro-system rollout status deploy/kro --timeout=180s

k apply -f "${KRO_RBAC_WORKAROUND_MANIFEST}"

echo "==> Applying infra RGD and waiting for ResourceGraphAccepted"
k apply -f "${KRO_RGD_MANIFEST}"
k wait --for=condition=ResourceGraphAccepted --timeout=120s "rgd/${RGD_NAME}"
k get crd "${RGD_INSTANCE_CRD}" -o name

echo "==> Installing Kany8s CRDs"
make install

echo "==> Building controller image ${IMG}"
make docker-build IMG="${IMG}"

echo "==> Loading controller image into kind cluster"
kind load docker-image "${IMG}" --name "${KIND_CLUSTER_NAME}"

echo "==> Deploying Kany8s controller-manager"
backup_kustomization
make deploy IMG="${IMG}"
k -n kany8s-system rollout status deployment/kany8s-controller-manager --timeout=180s

echo "==> Applying Kany8sCluster"

rendered_cluster_manifest="${ARTIFACTS_DIR}/kany8scluster.yaml"
sed \
	-e "s/__CLUSTER_NAME__/${CLUSTER_NAME}/g" \
	-e "s/__NAMESPACE__/${NAMESPACE}/g" \
	-e "s/__RGD_NAME__/${RGD_NAME}/g" \
	"${KANY8S_CLUSTER_TEMPLATE}" >"${rendered_cluster_manifest}"

k apply -f "${rendered_cluster_manifest}"

echo "==> Waiting for Kany8sCluster Ready"
k -n "${NAMESPACE}" wait --for=condition=Ready --timeout=240s "kany8scluster/${CLUSTER_NAME}"

echo "==> Waiting for Kany8sCluster provisioned"
k -n "${NAMESPACE}" wait --for=jsonpath='{.status.initialization.provisioned}'=true --timeout=240s "kany8scluster/${CLUSTER_NAME}"

echo "==> Verifying Kany8sCluster has no failure fields"
failure_reason="$(k -n "${NAMESPACE}" get kany8scluster "${CLUSTER_NAME}" -o jsonpath='{.status.failureReason}')"
failure_message="$(k -n "${NAMESPACE}" get kany8scluster "${CLUSTER_NAME}" -o jsonpath='{.status.failureMessage}')"

failure_reason="${failure_reason//$'\n'/}"
failure_reason="${failure_reason//$'\r'/}"
failure_message="${failure_message//$'\n'/}"
failure_message="${failure_message//$'\r'/}"

if [[ -n "${failure_reason}" && "${failure_reason}" != "<no value>" ]]; then
	echo "error: kany8scluster/${CLUSTER_NAME} has failureReason: ${failure_reason}" >&2
	exit 1
fi
if [[ -n "${failure_message}" && "${failure_message}" != "<no value>" ]]; then
	echo "error: kany8scluster/${CLUSTER_NAME} has failureMessage: ${failure_message}" >&2
	exit 1
fi

echo "error: kro infra reflection acceptance script is not fully implemented yet" >&2
echo "see docs/issues/kany8cluster-at-todo.md" >&2
exit 1
