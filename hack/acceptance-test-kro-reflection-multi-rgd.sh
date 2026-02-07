#!/usr/bin/env bash
set -euo pipefail

timestamp="$(date +%Y%m%d%H%M%S)"

KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-kany8s-acceptance-multi-rgd-${timestamp}}"
KUBECTL_CONTEXT="${KUBECTL_CONTEXT:-kind-${KIND_CLUSTER_NAME}}"
KRO_VERSION="${KRO_VERSION:-0.7.1}"

IMG="${IMG:-example.com/kany8s:acceptance-multi-rgd}"
KUBERNETES_VERSION="${KUBERNETES_VERSION:-1.34}"

NAMESPACE="${NAMESPACE:-default}"
CLUSTER_NAME="${CLUSTER_NAME:-demo-cluster}"
CLUSTER_NAME_ALT="${CLUSTER_NAME_ALT:-${CLUSTER_NAME}-alt}"

CLEANUP="${CLEANUP:-true}"

RGD_BASE_NAME="demo-control-plane.kro.run"
RGD_ALT_NAME="demo-control-plane-alt.kro.run"

RGD_BASE_INSTANCE_CRD="democontrolplanes.kro.run"
RGD_ALT_INSTANCE_CRD="democontrolplanealts.kro.run"

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

KRO_RBAC_WORKAROUND_MANIFEST="${KRO_RBAC_WORKAROUND_MANIFEST:-${repo_root}/test/acceptance_test/manifests/kro/rbac-unrestricted.yaml}"
KRO_RGD_BASE_MANIFEST="${KRO_RGD_BASE_MANIFEST:-${repo_root}/test/acceptance_test/manifests/kro/rgd.yaml}"
KRO_RGD_ALT_MANIFEST="${KRO_RGD_ALT_MANIFEST:-${repo_root}/test/acceptance_test/manifests/kro/multi-rgd/rgd-alt.yaml}"
KANY8S_CONTROLPLANE_TEMPLATE="${KANY8S_CONTROLPLANE_TEMPLATE:-${repo_root}/test/acceptance_test/manifests/kro/kany8scontrolplane.yaml.tpl}"

cd "${repo_root}"

ARTIFACTS_DIR="${ARTIFACTS_DIR:-/tmp/kany8s-acceptance-multi-rgd-${timestamp}}"
KUBECONFIG_FILE="${KUBECONFIG_FILE:-${ARTIFACTS_DIR}/kubeconfig}"

mkdir -p "${ARTIFACTS_DIR}"
log_file="${ARTIFACTS_DIR}/acceptance-multi-rgd.log"
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

	k get nodes -o wide >"${diag_dir}/nodes.txt" 2>&1 || true
	k get events -A --sort-by=.metadata.creationTimestamp >"${diag_dir}/events.txt" 2>&1 || true

	k -n kro-system get all -o wide >"${diag_dir}/kro-system.txt" 2>&1 || true
	k -n kro-system logs deploy/kro --tail=200 >"${diag_dir}/kro-logs.txt" 2>&1 || true

	k get rgd "${RGD_BASE_NAME}" -o yaml >"${diag_dir}/rgd-base.yaml" 2>&1 || true
	k get rgd "${RGD_ALT_NAME}" -o yaml >"${diag_dir}/rgd-alt.yaml" 2>&1 || true
	k get crd "${RGD_BASE_INSTANCE_CRD}" -o yaml >"${diag_dir}/rgd-base-instance-crd.yaml" 2>&1 || true
	k get crd "${RGD_ALT_INSTANCE_CRD}" -o yaml >"${diag_dir}/rgd-alt-instance-crd.yaml" 2>&1 || true

	k -n kany8s-system get all -o wide >"${diag_dir}/kany8s-system.txt" 2>&1 || true
	k -n kany8s-system logs deploy/kany8s-controller-manager -c manager --tail=200 >"${diag_dir}/kany8s-controller-logs.txt" 2>&1 || true

	k -n "${NAMESPACE}" get kany8scontrolplane "${CLUSTER_NAME}" -o yaml >"${diag_dir}/kany8scontrolplane-base.yaml" 2>&1 || true
	k -n "${NAMESPACE}" get kany8scontrolplane "${CLUSTER_NAME_ALT}" -o yaml >"${diag_dir}/kany8scontrolplane-alt.yaml" 2>&1 || true

	k -n "${NAMESPACE}" get "${RGD_BASE_INSTANCE_CRD}" "${CLUSTER_NAME}" -o yaml >"${diag_dir}/kro-instance-base.yaml" 2>&1 || true
	k -n "${NAMESPACE}" get "${RGD_ALT_INSTANCE_CRD}" "${CLUSTER_NAME_ALT}" -o yaml >"${diag_dir}/kro-instance-alt.yaml" 2>&1 || true
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

verify_control_plane() {
	local cp_name instance_crd
	cp_name="$1"
	instance_crd="$2"

	echo "==> Waiting for Kany8sControlPlane Ready=True: ${NAMESPACE}/${cp_name}"
	k -n "${NAMESPACE}" wait --for=condition=Ready --timeout=240s "kany8scontrolplane/${cp_name}"

	echo "==> Waiting for kro instance to exist: ${NAMESPACE}/${cp_name} (${instance_crd})"
	for _ in $(seq 1 240); do
		if k -n "${NAMESPACE}" get "${instance_crd}" "${cp_name}" >/dev/null 2>&1; then
			break
		fi
		sleep 1
	done

	echo "==> Waiting for kro instance status.ready=true: ${instance_crd}/${cp_name}"
	k -n "${NAMESPACE}" wait --for=jsonpath='{.status.ready}'=true --timeout=180s "${instance_crd}/${cp_name}"

	endpoint="$(k -n "${NAMESPACE}" get "${instance_crd}" "${cp_name}" -o jsonpath='{.status.endpoint}')"
	if [[ -z "${endpoint}" ]]; then
		echo "error: kro instance status.endpoint is empty (${instance_crd}/${cp_name})" >&2
		exit 1
	fi

	cp_host="$(k -n "${NAMESPACE}" get kany8scontrolplane "${cp_name}" -o jsonpath='{.spec.controlPlaneEndpoint.host}')"
	cp_port="$(k -n "${NAMESPACE}" get kany8scontrolplane "${cp_name}" -o jsonpath='{.spec.controlPlaneEndpoint.port}')"
	initialized="$(k -n "${NAMESPACE}" get kany8scontrolplane "${cp_name}" -o jsonpath='{.status.initialization.controlPlaneInitialized}')"

	if [[ -z "${cp_host}" ]]; then
		echo "error: Kany8sControlPlane.spec.controlPlaneEndpoint.host is empty (${cp_name})" >&2
		exit 1
	fi
	if [[ -z "${cp_port}" || "${cp_port}" == "0" ]]; then
		echo "error: Kany8sControlPlane.spec.controlPlaneEndpoint.port is empty/0 (${cp_name})" >&2
		exit 1
	fi
	if [[ "${initialized}" != "true" ]]; then
		echo "error: Kany8sControlPlane.status.initialization.controlPlaneInitialized expected true, got ${initialized} (${cp_name})" >&2
		exit 1
	fi

	no_scheme="${endpoint}"
	no_scheme="${no_scheme#https://}"
	expected_host="${no_scheme%%:*}"
	expected_port="443"
	if [[ "${no_scheme}" == *":"* ]]; then
		expected_port="${no_scheme##*:}"
	fi

	if [[ "${cp_host}" != "${expected_host}" ]]; then
		echo "error: endpoint host mismatch: got ${cp_host}, want ${expected_host} (endpoint=${endpoint}, cp=${cp_name})" >&2
		exit 1
	fi
	if [[ "${cp_port}" != "${expected_port}" ]]; then
		echo "error: endpoint port mismatch: got ${cp_port}, want ${expected_port} (endpoint=${endpoint}, cp=${cp_name})" >&2
		exit 1
	fi
}

need_cmd docker
need_cmd kind
need_cmd kubectl
need_cmd make
need_cmd go
need_cmd curl

echo "==> Creating kind cluster ${KIND_CLUSTER_NAME}"
kind create cluster --name "${KIND_CLUSTER_NAME}" --wait 60s --kubeconfig "${KUBECONFIG_FILE}"

echo "==> Verifying cluster is reachable"
k get nodes -o wide

echo "==> Installing kro v${KRO_VERSION}"
if ! k get namespace kro-system >/dev/null 2>&1; then
	k create namespace kro-system
fi

KRO_CORE_INSTALL_MANIFEST="${KRO_CORE_INSTALL_MANIFEST:-${repo_root}/test/acceptance_test/vendor/kro/v${KRO_VERSION}/kro-core-install-manifests.yaml}"
mkdir -p "$(dirname "${KRO_CORE_INSTALL_MANIFEST}")"

if [[ ! -f "${KRO_CORE_INSTALL_MANIFEST}" ]]; then
	echo "==> Downloading kro install manifest to ${KRO_CORE_INSTALL_MANIFEST}"
	curl -fsSL -o "${KRO_CORE_INSTALL_MANIFEST}" \
		"https://github.com/kubernetes-sigs/kro/releases/download/v${KRO_VERSION}/kro-core-install-manifests.yaml"
fi

k apply -f "${KRO_CORE_INSTALL_MANIFEST}"
k -n kro-system rollout status deploy/kro --timeout=180s

echo "==> Applying kro RBAC workaround (v0.7.1)"
k apply -f "${KRO_RBAC_WORKAROUND_MANIFEST}"

echo "==> Applying demo RGDs and waiting for ResourceGraphAccepted"
k apply -f "${KRO_RGD_BASE_MANIFEST}"
k apply -f "${KRO_RGD_ALT_MANIFEST}"

k wait --for=condition=ResourceGraphAccepted --timeout=120s "rgd/${RGD_BASE_NAME}"
k wait --for=condition=ResourceGraphAccepted --timeout=120s "rgd/${RGD_ALT_NAME}"

k get crd "${RGD_BASE_INSTANCE_CRD}" -o name
k get crd "${RGD_ALT_INSTANCE_CRD}" -o name

echo "==> Installing Kany8s CRDs"
make install

echo "==> Building controller image ${IMG}"
make docker-build IMG="${IMG}"

echo "==> Loading controller image into kind"
kind load docker-image "${IMG}" --name "${KIND_CLUSTER_NAME}"

echo "==> Deploying Kany8s controller-manager"
backup_kustomization
make deploy IMG="${IMG}"
k -n kany8s-system rollout status deployment/kany8s-controller-manager --timeout=180s

echo "==> Applying Kany8sControlPlane objects (two different RGDs)"

rendered_controlplane_base="${ARTIFACTS_DIR}/kany8scontrolplane-base.yaml"
sed \
	-e "s/__CLUSTER_NAME__/${CLUSTER_NAME}/g" \
	-e "s/__NAMESPACE__/${NAMESPACE}/g" \
	-e "s/__KUBERNETES_VERSION__/${KUBERNETES_VERSION}/g" \
	-e "s/__RGD_NAME__/${RGD_BASE_NAME}/g" \
	"${KANY8S_CONTROLPLANE_TEMPLATE}" >"${rendered_controlplane_base}"

rendered_controlplane_alt="${ARTIFACTS_DIR}/kany8scontrolplane-alt.yaml"
sed \
	-e "s/__CLUSTER_NAME__/${CLUSTER_NAME_ALT}/g" \
	-e "s/__NAMESPACE__/${NAMESPACE}/g" \
	-e "s/__KUBERNETES_VERSION__/${KUBERNETES_VERSION}/g" \
	-e "s/__RGD_NAME__/${RGD_ALT_NAME}/g" \
	"${KANY8S_CONTROLPLANE_TEMPLATE}" >"${rendered_controlplane_alt}"

k apply -f "${rendered_controlplane_base}"
k apply -f "${rendered_controlplane_alt}"

verify_control_plane "${CLUSTER_NAME}" "${RGD_BASE_INSTANCE_CRD}"
verify_control_plane "${CLUSTER_NAME_ALT}" "${RGD_ALT_INSTANCE_CRD}"

echo "==> OK: multi-RGD acceptance test passed"
