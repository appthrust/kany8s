#!/usr/bin/env bash
set -euo pipefail

timestamp="${TIMESTAMP:-$(date +%Y%m%d%H%M%S)}"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

KIND_CLUSTER="${KIND_CLUSTER:-kany8s-test-e2e}"
ARTIFACTS_DIR="${ARTIFACTS_DIR:-/tmp/kany8s-e2e-${timestamp}}"

# Restore `config/manager/kustomization.yaml` after the run because the e2e flow
# runs `make deploy` / `make build-installer` style commands which may edit it.
RESTORE_KUSTOMIZATION="${RESTORE_KUSTOMIZATION:-true}"

command -v kind >/dev/null 2>&1 || { echo "error: kind not found" >&2; exit 1; }
command -v make >/dev/null 2>&1 || { echo "error: make not found" >&2; exit 1; }
command -v curl >/dev/null 2>&1 || { echo "error: curl not found" >&2; exit 1; }

mkdir -p "${ARTIFACTS_DIR}"
log_file="${ARTIFACTS_DIR}/e2e.log"
exec > >(tee -a "${log_file}") 2>&1

echo "==> Artifacts"
echo "    ARTIFACTS_DIR=${ARTIFACTS_DIR}"
echo "    LOG_FILE=${log_file}"
echo "    KIND_CLUSTER=${KIND_CLUSTER}"

kustomization_path="${repo_root}/config/manager/kustomization.yaml"
kustomization_backup=""

backup_kustomization() {
	if [[ "${RESTORE_KUSTOMIZATION}" == "true" && -f "${kustomization_path}" ]]; then
		kustomization_backup="${ARTIFACTS_DIR}/kustomization.yaml.bak"
		cp "${kustomization_path}" "${kustomization_backup}"
	fi
}

restore_kustomization() {
	if [[ "${RESTORE_KUSTOMIZATION}" == "true" && -n "${kustomization_backup}" && -f "${kustomization_backup}" ]]; then
		cp "${kustomization_backup}" "${kustomization_path}"
	fi
}

cleanup() {
	restore_kustomization
	echo "==> Cleaning up kind cluster ${KIND_CLUSTER}"
	kind delete cluster --name "${KIND_CLUSTER}" >/dev/null 2>&1 || true
}

on_exit() {
	local rc
	rc="$?"
	cleanup
	exit "${rc}"
}

trap on_exit EXIT

echo "==> Pre-clean kind cluster: ${KIND_CLUSTER}"
kind delete cluster --name "${KIND_CLUSTER}" >/dev/null 2>&1 || true

cert_manager_manifest="${repo_root}/test/acceptance_test/vendor/cert-manager/v1.19.2/cert-manager.yaml"
mkdir -p "$(dirname "${cert_manager_manifest}")"
if [[ ! -f "${cert_manager_manifest}" ]]; then
	echo "==> Downloading cert-manager manifest to ${cert_manager_manifest}"
	curl -fsSL -o "${cert_manager_manifest}" \
		"https://github.com/cert-manager/cert-manager/releases/download/v1.19.2/cert-manager.yaml"
fi

backup_kustomization

echo "==> Running e2e"
KIND_CLUSTER="${KIND_CLUSTER}" CERT_MANAGER_MANIFEST="${cert_manager_manifest}" make test-e2e
