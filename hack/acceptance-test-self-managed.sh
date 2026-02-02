#!/usr/bin/env bash
set -euo pipefail

timestamp="$(date +%Y%m%d%H%M%S)"

KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-kany8s-acceptance-self-managed-${timestamp}}"
KUBECTL_CONTEXT="${KUBECTL_CONTEXT:-kind-${KIND_CLUSTER_NAME}}"

IMG="${IMG:-example.com/kany8s:acceptance-self-managed}"
CLUSTERCTL_VERSION="${CLUSTERCTL_VERSION:-v1.12.2}"

DOCKER_SOCK="${DOCKER_SOCK:-/var/run/docker.sock}"

NAMESPACE="${NAMESPACE:-default}"
CLUSTER_NAME="${CLUSTER_NAME:-demo-self-managed-docker}"
CLUSTER_WAIT_TIMEOUT="${CLUSTER_WAIT_TIMEOUT:-30m}"

CLEANUP="${CLEANUP:-true}"

# If false, we'll refuse to run if workload Docker containers already exist for CLUSTER_NAME.
# Reusing old containers almost always breaks RemoteConnectionProbe because the kubeconfig CA won't match.
REUSE_EXISTING_WORKLOAD_DOCKER_CONTAINERS="${REUSE_EXISTING_WORKLOAD_DOCKER_CONTAINERS:-false}"

ARTIFACTS_DIR="${ARTIFACTS_DIR:-/tmp/kany8s-acceptance-self-managed-${timestamp}}"
KUBECONFIG_FILE="${KUBECONFIG_FILE:-${ARTIFACTS_DIR}/kubeconfig}"
WORKLOAD_KUBECONFIG_FILE="${WORKLOAD_KUBECONFIG_FILE:-${ARTIFACTS_DIR}/workload.kubeconfig}"

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "${repo_root}"

mkdir -p "${ARTIFACTS_DIR}"
log_file="${ARTIFACTS_DIR}/acceptance-self-managed.log"
exec > >(tee -a "${log_file}") 2>&1

echo "==> Artifacts"
echo "    ARTIFACTS_DIR=${ARTIFACTS_DIR}"
echo "    LOG_FILE=${log_file}"
echo "    KUBECONFIG=${KUBECONFIG_FILE}"
echo "    KIND_CLUSTER_NAME=${KIND_CLUSTER_NAME}"
echo "    KUBECTL_CONTEXT=${KUBECTL_CONTEXT}"
echo "    NAMESPACE=${NAMESPACE}"
echo "    CLUSTER_NAME=${CLUSTER_NAME}"
echo "    CLEANUP=${CLEANUP}"
echo ""
echo "==> Useful (while running)"
echo "    tail -f ${log_file}"
echo "    kubectl --kubeconfig ${KUBECONFIG_FILE} --context ${KUBECTL_CONTEXT} get pods -A"
echo "    kubectl --kubeconfig ${KUBECONFIG_FILE} --context ${KUBECTL_CONTEXT} -n ${NAMESPACE} get cluster ${CLUSTER_NAME} -o wide"
echo "    kubectl --kubeconfig ${KUBECONFIG_FILE} --context ${KUBECTL_CONTEXT} -n ${NAMESPACE} describe cluster ${CLUSTER_NAME}"
echo ""

# kind uses "${KIND_CLUSTER_NAME}-control-plane" as node container name and hostname.
# Linux hostnames are typically limited to 63 chars.
hostname_suffix="-control-plane"
max_cluster_name_len=$((63 - ${#hostname_suffix}))
if [[ ${#KIND_CLUSTER_NAME} -gt ${max_cluster_name_len} ]]; then
	echo "error: KIND_CLUSTER_NAME is too long (${#KIND_CLUSTER_NAME} chars); must be <= ${max_cluster_name_len} to keep hostnames <= 63" >&2
	echo "  KIND_CLUSTER_NAME=${KIND_CLUSTER_NAME}" >&2
	echo "hint: set a shorter KIND_CLUSTER_NAME (e.g. kany8s-sm-<timestamp>)" >&2
	exit 1
fi

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

duration_to_seconds() {
	local raw
	raw="$1"
	case "${raw}" in
		"" ) echo 0; return 0 ;;
		*[0-9]s ) echo "${raw%s}"; return 0 ;;
		*[0-9]m ) echo "$(( ${raw%m} * 60 ))"; return 0 ;;
		*[0-9]h ) echo "$(( ${raw%h} * 3600 ))"; return 0 ;;
		*[0-9] ) echo "${raw}"; return 0 ;;
		* ) echo 0; return 0 ;;
	esac
}

workload_docker_containers() {
	# Workload cluster containers are created on the host Docker daemon via CAPD.
	# They are NOT removed by deleting the kind management cluster.
	docker ps -a --format '{{.Names}}' | while read -r name; do
		if [[ "${name}" == "${CLUSTER_NAME}-"* ]]; then
			echo "${name}"
		fi
	done
}

cleanup_workload_docker_containers() {
	local found
	found="$(workload_docker_containers || true)"
	if [[ -z "${found}" ]]; then
		return 0
	fi

	echo "==> Cleaning up workload Docker containers for ${CLUSTER_NAME}"
	echo "${found}" | while read -r name; do
		if [[ -n "${name}" ]]; then
			docker rm -f "${name}" >/dev/null 2>&1 || true
		fi
	done
}

wait_cluster_condition_with_progress() {
	local condition
	condition="$1"
	local timeout_raw
	timeout_raw="$2"
	local timeout_seconds
	timeout_seconds="$(duration_to_seconds "${timeout_raw}")"
	if [[ "${timeout_seconds}" -le 0 ]]; then
		timeout_seconds=1800
	fi

	local start
	start="$(date +%s)"

	while true; do
		local status
		status="$(k -n "${NAMESPACE}" get cluster "${CLUSTER_NAME}" -o jsonpath='{.status.conditions[?(@.type=="'"${condition}"'")].status}' 2>/dev/null || true)"
		if [[ "${status}" == "True" ]]; then
			echo "==> Cluster ${condition}=True"
			return 0
		fi

		local now
		now="$(date +%s)"
		if (( now - start >= timeout_seconds )); then
			echo "error: timeout waiting for Cluster ${condition}=True (timeout=${timeout_raw})" >&2
			k -n "${NAMESPACE}" describe cluster "${CLUSTER_NAME}" || true
			return 1
		fi

		echo "==> Waiting for Cluster ${condition}=True (elapsed=$(( now - start ))s / timeout=${timeout_raw})"
		k -n "${NAMESPACE}" get cluster "${CLUSTER_NAME}" -o wide || true
		k -n "${NAMESPACE}" get kany8skubeadmcontrolplane "${CLUSTER_NAME}" -o wide 2>/dev/null || true
		k -n "${NAMESPACE}" get secret "${CLUSTER_NAME}-kubeconfig" -o name 2>/dev/null || true
		k -n "${NAMESPACE}" get machine -l "cluster.x-k8s.io/cluster-name=${CLUSTER_NAME}" -o wide 2>/dev/null || true
		sleep 15
	done
}

wait_rollout() {
	local ns name timeout_raw
	ns="$1"
	name="$2"
	timeout_raw="$3"
	echo "==> Waiting for rollout ${ns}/${name}"
	k -n "${ns}" rollout status "${name}" --timeout="${timeout_raw}"
}

wait_service_endpoints() {
	local ns svc timeout_raw
	ns="$1"
	svc="$2"
	timeout_raw="$3"
	local timeout_seconds start now
	timeout_seconds="$(duration_to_seconds "${timeout_raw}")"
	if [[ "${timeout_seconds}" -le 0 ]]; then
		timeout_seconds=300
	fi
	start="$(date +%s)"
	while true; do
		local eps
		eps="$(k -n "${ns}" get endpoints "${svc}" -o jsonpath='{range .subsets[*]}{range .addresses[*]}{.ip}{"\n"}{end}{end}' 2>/dev/null || true)"
		if [[ -n "${eps}" ]]; then
			echo "==> Service endpoints ready: ${ns}/${svc}"
			return 0
		fi
		now="$(date +%s)"
		if (( now - start >= timeout_seconds )); then
			echo "error: timeout waiting for endpoints ${ns}/${svc} (timeout=${timeout_raw})" >&2
			k -n "${ns}" get svc "${svc}" -o wide || true
			k -n "${ns}" get endpoints "${svc}" -o yaml || true
			k -n "${ns}" get pods -o wide || true
			return 1
		fi
		sleep 3
	done
}

apply_manifest_with_webhook_retry() {
	local path attempts delay
	path="$1"
	attempts="${2:-30}"
	delay="${3:-3}"

	local i
	for i in $(seq 1 "${attempts}"); do
		set +e
		out="$(k apply -f "${path}" 2>&1)"
		rc=$?
		set -e

		if [[ "${rc}" -eq 0 ]]; then
			echo "${out}"
			return 0
		fi

		echo "${out}"
		if [[ "${out}" == *"failed calling webhook"* && "${out}" == *"connection refused"* ]]; then
			echo "==> Webhook connection refused; retrying (${i}/${attempts}) after ${delay}s"
			sleep "${delay}"
			continue
		fi

		return "${rc}"
	done

	echo "error: giving up applying manifest after ${attempts} attempts: ${path}" >&2
	return 1
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
		cleanup_workload_docker_containers
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

existing_workload_containers="$(workload_docker_containers || true)"
if [[ -n "${existing_workload_containers}" && "${REUSE_EXISTING_WORKLOAD_DOCKER_CONTAINERS}" != "true" ]]; then
	echo "error: workload Docker containers already exist for CLUSTER_NAME=${CLUSTER_NAME}" >&2
	echo "This typically causes RemoteConnectionProbe to fail due to CA mismatch." >&2
	echo "Existing containers:" >&2
	echo "${existing_workload_containers}" >&2
	echo "" >&2
	echo "Fix options:" >&2
	echo "  1) Remove them: docker rm -f <names listed above>" >&2
	echo "  2) If you REALLY intend to reuse them: set REUSE_EXISTING_WORKLOAD_DOCKER_CONTAINERS=true" >&2
	exit 1
fi

echo "==> Creating kind management cluster ${KIND_CLUSTER_NAME}"
if [[ ! -S "${DOCKER_SOCK}" ]]; then
	echo "error: docker socket not found (expected unix socket): ${DOCKER_SOCK}" >&2
	exit 1
fi

kind_config_file="${ARTIFACTS_DIR}/kind-config.yaml"
cat >"${kind_config_file}" <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    extraMounts:
      - hostPath: ${DOCKER_SOCK}
        containerPath: /var/run/docker.sock
EOF

kind create cluster \
	--name "${KIND_CLUSTER_NAME}" \
	--wait 60s \
	--kubeconfig "${KUBECONFIG_FILE}" \
	--config "${kind_config_file}"

echo "==> Verifying docker.sock is mounted into kind node"
kind_nodes="$(kind get nodes --name "${KIND_CLUSTER_NAME}")"
kind_node="${kind_nodes%%$'\n'*}"
docker exec "${kind_node}" test -S /var/run/docker.sock

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

# clusterctl's local filesystem repository expects a layout like:
# {basepath}/{provider-name}/{version}/{components.yaml}
# so we stage the bundle into a versioned directory.
provider_version="${KANY8S_PROVIDER_VERSION:-v0.0.0}"
provider_label="control-plane-kany8s"
local_repo_dir="${ARTIFACTS_DIR}/clusterctl-repository/${provider_label}/${provider_version}"
mkdir -p "${local_repo_dir}"
local_bundle_path="${local_repo_dir}/install.yaml"
cp "${bundle_path}" "${local_bundle_path}"
bundle_path="$(realpath "${local_bundle_path}")"

version_no_v="${provider_version#v}"
major="${version_no_v%%.*}"
minor_tmp="${version_no_v#*.}"
minor="${minor_tmp%%.*}"

cat >"${local_repo_dir}/metadata.yaml" <<EOF
apiVersion: clusterctl.cluster.x-k8s.io/v1alpha3
kind: Metadata
releaseSeries:
  - major: ${major}
    minor: ${minor}
    contract: v1beta2
EOF
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

echo "==> Waiting for CAPD webhook to be ready"
wait_rollout "capd-system" "deploy/capd-controller-manager" "5m"
wait_service_endpoints "capd-system" "capd-webhook-service" "5m"

# give kube-proxy a moment to program Service rules
sleep 2

echo "==> Applying self-managed sample manifests"
apply_manifest_with_webhook_retry "${repo_root}/examples/self-managed-docker/cluster.yaml"

echo "==> Waiting for Cluster RemoteConnectionProbe=True"
wait_cluster_condition_with_progress "RemoteConnectionProbe" "${CLUSTER_WAIT_TIMEOUT}"

echo "==> Waiting for Cluster Available=True"
wait_cluster_condition_with_progress "Available" "${CLUSTER_WAIT_TIMEOUT}"

echo "==> Fetching workload kubeconfig"
echo "==> clusterctl get kubeconfig -n ${NAMESPACE} ${CLUSTER_NAME}"
"${clusterctl_bin}" --config "${clusterctl_config}" get kubeconfig -n "${NAMESPACE}" "${CLUSTER_NAME}" >"${WORKLOAD_KUBECONFIG_FILE}"

echo "==> Verifying workload cluster connectivity"
kubectl --kubeconfig "${WORKLOAD_KUBECONFIG_FILE}" get nodes -o wide

echo "==> OK: self-managed acceptance test passed"
