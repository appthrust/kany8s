#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="${NAMESPACE:-default}"
CLUSTER_NAME="${CLUSTER_NAME:-}"
NETWORK_NAME="${NETWORK_NAME:-}"

KUBECTL_CONTEXT="${KUBECTL_CONTEXT:-}"

DELETE_NETWORK="${DELETE_NETWORK:-true}"
WAIT_TIMEOUT_EKS="${WAIT_TIMEOUT_EKS:-40m}"
WAIT_TIMEOUT_MANAGED="${WAIT_TIMEOUT_MANAGED:-25m}"
WAIT_TIMEOUT_NETWORK="${WAIT_TIMEOUT_NETWORK:-30m}"
WAIT_TIMEOUT_AWS="${WAIT_TIMEOUT_AWS:-40m}"
POLL_INTERVAL="${POLL_INTERVAL:-15s}"

AWS_REGION="${AWS_REGION:-}"

CONFIRM="${CONFIRM:-false}"

EKS_CLUSTER_NAME=""

if [[ -z "${CLUSTER_NAME}" ]]; then
	echo "error: CLUSTER_NAME is required" >&2
	echo "example: CLUSTER_NAME=demo-eks-byo-135-20260208121023 CONFIRM=true bash hack/eks-fargate-dev-reset.sh" >&2
	exit 1
fi

if [[ -z "${NETWORK_NAME}" ]]; then
	NETWORK_NAME="${CLUSTER_NAME}-net"
fi

k() {
	if [[ -n "${KUBECTL_CONTEXT}" ]]; then
		kubectl --context "${KUBECTL_CONTEXT}" "$@"
		return
	fi
	kubectl "$@"
}

info() {
	echo "==> $*"
}

warn() {
	echo "warning: $*" >&2
}

duration_to_seconds() {
	local duration="${1:-}"
	if [[ "${duration}" =~ ^([0-9]+)s$ ]]; then
		echo "${BASH_REMATCH[1]}"
		return 0
	fi
	if [[ "${duration}" =~ ^([0-9]+)m$ ]]; then
		echo "$((BASH_REMATCH[1] * 60))"
		return 0
	fi
	if [[ "${duration}" =~ ^([0-9]+)h$ ]]; then
		echo "$((BASH_REMATCH[1] * 3600))"
		return 0
	fi
	echo "unsupported duration format: ${duration} (use Ns/Nm/Nh)" >&2
	return 1
}

list_bootstrapper_managed_resources() {
	k -n "${NAMESPACE}" get \
		openidconnectproviders.iam.services.k8s.aws,roles.iam.services.k8s.aws,policies.iam.services.k8s.aws,instanceprofiles.iam.services.k8s.aws,\
		accessentries.eks.services.k8s.aws,fargateprofiles.eks.services.k8s.aws,securitygroups.ec2.services.k8s.aws,\
		ocirepositories.source.toolkit.fluxcd.io,helmreleases.helm.toolkit.fluxcd.io,\
		configmaps,secrets,clusterresourcesets.addons.cluster.x-k8s.io \
		-l cluster.x-k8s.io/cluster-name="${CLUSTER_NAME}" \
		-o name 2>/dev/null || true
}

print_delete_diagnostics() {
	local resources="${1:-}"
	if [[ -z "${resources}" ]]; then
		return 0
	fi

	warn "some managed resources are still present; collecting deletion diagnostics"

	print_sg_dependency_diagnostics() {
		local resource="${1:-}"
		[[ -z "${resource}" ]] && return 0
		[[ "${resource}" != securitygroups.ec2.services.k8s.aws/* ]] && return 0

		local sg_cr_name sg_id region out orphan_enis
		sg_cr_name="${resource#securitygroups.ec2.services.k8s.aws/}"
		[[ -z "${sg_cr_name}" || "${sg_cr_name}" == "${resource}" ]] && return 0

		sg_id="$(k -n "${NAMESPACE}" get securitygroups.ec2.services.k8s.aws "${sg_cr_name}" -o jsonpath='{.status.id}' 2>/dev/null || true)"
		sg_id="${sg_id//[[:space:]]/}"
		if [[ -z "${sg_id}" ]]; then
			warn "security group DependencyViolation detected but .status.id is missing for ${resource}"
			return 0
		fi

		region="${AWS_REGION}"
		if [[ -z "${region}" ]]; then
			region="$(k -n "${NAMESPACE}" get securitygroups.ec2.services.k8s.aws "${sg_cr_name}" -o jsonpath='{.status.ackResourceMetadata.region}' 2>/dev/null || true)"
		fi
		if [[ -z "${region}" ]]; then
			region="$(k -n "${NAMESPACE}" get securitygroups.ec2.services.k8s.aws "${sg_cr_name}" -o jsonpath="{.metadata.annotations['services.k8s.aws/region']}" 2>/dev/null || true)"
		fi
		region="${region//[[:space:]]/}"
		if [[ -z "${region}" ]]; then
			warn "could not resolve AWS region for ${resource}; set AWS_REGION=<region> to inspect dependent ENIs"
			return 0
		fi
		if ! command -v aws >/dev/null 2>&1; then
			warn "aws CLI is not installed; cannot inspect dependent ENIs for ${resource}"
			return 0
		fi

		warn "SecurityGroup DependencyViolation diagnostics (resource=${resource}, region=${region}, sg_id=${sg_id})"
		if out="$({ aws ec2 describe-network-interfaces \
			--region "${region}" \
			--filters Name=group-id,Values="${sg_id}" \
			--query 'NetworkInterfaces[].{id:NetworkInterfaceId,status:Status,attachment:Attachment,desc:Description,tags:TagSet}' \
			--output yaml; } 2>&1)"; then
			echo "${out}" >&2
		else
			warn "aws ec2 describe-network-interfaces failed: ${out}"
			return 0
		fi

		orphan_enis="$({ aws ec2 describe-network-interfaces \
			--region "${region}" \
			--filters Name=group-id,Values="${sg_id}" Name=tag:eks:eni:owner,Values=amazon-vpc-cni \
			--query "NetworkInterfaces[?Attachment==\`null\` && Status=='available'].NetworkInterfaceId" \
			--output text; } 2>/dev/null || true)"
		orphan_enis="$(tr -s '[:space:]' ' ' <<<"${orphan_enis}" | sed -e 's/^ //g' -e 's/ $//g')"
		if [[ -n "${orphan_enis}" ]]; then
			warn "candidate orphan ENIs (Attachment=null, Status=available, eks:eni:owner=amazon-vpc-cni): ${orphan_enis}"
			warn "break-glass delete commands (run only after cluster/node termination is confirmed):"
			for eni in ${orphan_enis}; do
				echo "aws ec2 delete-network-interface --region \"${region}\" --network-interface-id \"${eni}\"" >&2
			done
		fi
	}

	while IFS= read -r resource; do
		[[ -z "${resource}" ]] && continue
		finalizers="$(k -n "${NAMESPACE}" get "${resource}" -o jsonpath='{.metadata.finalizers}' 2>/dev/null || true)"
		if [[ -n "${finalizers}" && "${finalizers}" != "[]" ]]; then
			warn "stuck finalizer: ${resource} finalizers=${finalizers}"
		fi

		describe_out="$(k -n "${NAMESPACE}" describe "${resource}" 2>/dev/null || true)"
		if [[ -n "${describe_out}" ]] && grep -Eq 'DeleteConflict|DependencyViolation' <<<"${describe_out}"; then
			warn "delete conflict hint for ${resource}:"
			grep -E 'DeleteConflict|DependencyViolation' <<<"${describe_out}" >&2 || true
			if grep -q 'DependencyViolation' <<<"${describe_out}"; then
				print_sg_dependency_diagnostics "${resource}" || true
			fi
		fi
	done <<<"${resources}"
}

wait_for_managed_resources_deletion() {
	local timeout="${1:?timeout is required}"
	local timeout_seconds
	timeout_seconds="$(duration_to_seconds "${timeout}")"
	local start_ts
	start_ts="$(date +%s)"

	while true; do
		local resources remaining count now_ts elapsed
		resources="$(list_bootstrapper_managed_resources)"
		remaining="$(sed '/^$/d' <<<"${resources}")"
		if [[ -z "${remaining}" ]]; then
			info "bootstrapper-managed resources are deleted"
			return 0
		fi

		count="$(wc -l <<<"${remaining}" | tr -d ' ')"
		now_ts="$(date +%s)"
		elapsed="$((now_ts - start_ts))"
		if ((elapsed >= timeout_seconds)); then
			warn "timed out waiting for managed resources deletion (timeout=${timeout}, remaining=${count})"
			echo "${remaining}" >&2
			print_delete_diagnostics "${remaining}"
			return 1
		fi

		info "waiting for managed resources deletion (${count} remaining)"
		sleep "${POLL_INTERVAL}"
	done
}

resolve_eks_identifiers() {
	EKS_CLUSTER_NAME="$(k -n "${NAMESPACE}" get cluster.cluster.x-k8s.io "${CLUSTER_NAME}" -o jsonpath="{.metadata.annotations['eks.kany8s.io/cluster-name']}" 2>/dev/null || true)"
	if [[ -z "${EKS_CLUSTER_NAME}" ]]; then
		EKS_CLUSTER_NAME="$(k -n "${NAMESPACE}" get cluster.cluster.x-k8s.io "${CLUSTER_NAME}" -o jsonpath='{.spec.controlPlaneRef.name}' 2>/dev/null || true)"
	fi
	if [[ -z "${EKS_CLUSTER_NAME}" ]]; then
		EKS_CLUSTER_NAME="${CLUSTER_NAME}"
	fi
	if [[ -z "${AWS_REGION}" ]]; then
		AWS_REGION="$(k -n "${NAMESPACE}" get cluster.cluster.x-k8s.io "${CLUSTER_NAME}" -o jsonpath="{.metadata.annotations['eks.kany8s.io/region']}" 2>/dev/null || true)"
	fi
}

wait_for_aws_eks_disappearance() {
	local timeout="${1:?timeout is required}"
	if ! command -v aws >/dev/null 2>&1; then
		info "skip AWS-side verification (aws CLI is not installed)"
		return 0
	fi
	if [[ -z "${AWS_REGION}" ]]; then
		info "skip AWS-side verification (AWS_REGION could not be resolved)"
		return 0
	fi
	if [[ -z "${EKS_CLUSTER_NAME}" ]]; then
		info "skip AWS-side verification (EKS cluster name could not be resolved)"
		return 0
	fi

	local timeout_seconds
	timeout_seconds="$(duration_to_seconds "${timeout}")"
	local start_ts
	start_ts="$(date +%s)"

	while true; do
		local now_ts elapsed output
		if output="$({ aws eks describe-cluster --region "${AWS_REGION}" --name "${EKS_CLUSTER_NAME}" >/dev/null; } 2>&1)"; then
			:
		else
			if grep -q "ResourceNotFoundException" <<<"${output}"; then
				info "AWS EKS cluster is deleted (region=${AWS_REGION}, name=${EKS_CLUSTER_NAME})"
				return 0
			fi
			warn "aws eks describe-cluster returned a non-NotFound error: ${output}"
		fi

		now_ts="$(date +%s)"
		elapsed="$((now_ts - start_ts))"
		if ((elapsed >= timeout_seconds)); then
			warn "timed out waiting for AWS EKS cluster deletion (region=${AWS_REGION}, name=${EKS_CLUSTER_NAME})"
			return 1
		fi
		info "waiting for AWS EKS cluster deletion (region=${AWS_REGION}, name=${EKS_CLUSTER_NAME})"
		sleep "${POLL_INTERVAL}"
	done
}

if [[ "${CONFIRM}" != "true" ]]; then
	echo "This script deletes Kubernetes (and optionally AWS via ACK) resources." >&2
	echo "Set CONFIRM=true to proceed." >&2
	echo >&2
	echo "Planned actions:" >&2
	echo "- kubectl -n ${NAMESPACE} delete cluster.cluster.x-k8s.io/${CLUSTER_NAME}" >&2
	echo "- wait for bootstrapper-managed resources deletion (timeout=${WAIT_TIMEOUT_MANAGED})" >&2
	echo "- verify AWS EKS cluster disappearance when aws CLI is available (timeout=${WAIT_TIMEOUT_AWS})" >&2
	if [[ "${DELETE_NETWORK}" == "true" ]]; then
		echo "- kubectl -n ${NAMESPACE} delete ACK EC2 network resources for ${NETWORK_NAME}-*" >&2
	fi
	exit 2
fi

resolve_eks_identifiers

info "Current resources (bootstrapper-managed)"
list_bootstrapper_managed_resources || true

info "Deleting CAPI Cluster ${NAMESPACE}/${CLUSTER_NAME}"
k -n "${NAMESPACE}" delete cluster.cluster.x-k8s.io "${CLUSTER_NAME}" --ignore-not-found

info "Waiting for CAPI Cluster deletion (timeout=${WAIT_TIMEOUT_EKS})"
k -n "${NAMESPACE}" wait --for=delete --timeout="${WAIT_TIMEOUT_EKS}" cluster.cluster.x-k8s.io/"${CLUSTER_NAME}" || true

info "Waiting for bootstrapper-managed resources deletion (timeout=${WAIT_TIMEOUT_MANAGED})"
wait_for_managed_resources_deletion "${WAIT_TIMEOUT_MANAGED}" || true

if [[ "${DELETE_NETWORK}" == "true" ]]; then
	info "Deleting ACK EC2 network resources for ${NETWORK_NAME} (timeout=${WAIT_TIMEOUT_NETWORK})"

	# Best-effort: delete all known resources; ACK controllers will resolve dependencies.
	k -n "${NAMESPACE}" delete internetgateways.ec2.services.k8s.aws "${NETWORK_NAME}-igw" --ignore-not-found || true
	k -n "${NAMESPACE}" delete natgateways.ec2.services.k8s.aws "${NETWORK_NAME}-natgw" --ignore-not-found || true
	k -n "${NAMESPACE}" delete elasticipaddresses.ec2.services.k8s.aws "${NETWORK_NAME}-eip-nat" --ignore-not-found || true
	
	k -n "${NAMESPACE}" delete routetables.ec2.services.k8s.aws "${NETWORK_NAME}-rtb-public" --ignore-not-found || true
	k -n "${NAMESPACE}" delete routetables.ec2.services.k8s.aws "${NETWORK_NAME}-rtb-private" --ignore-not-found || true
	
	k -n "${NAMESPACE}" delete subnets.ec2.services.k8s.aws "${NETWORK_NAME}-subnet-public-a" --ignore-not-found || true
	k -n "${NAMESPACE}" delete subnets.ec2.services.k8s.aws "${NETWORK_NAME}-subnet-private-a" --ignore-not-found || true
	k -n "${NAMESPACE}" delete subnets.ec2.services.k8s.aws "${NETWORK_NAME}-subnet-private-b" --ignore-not-found || true
	
	k -n "${NAMESPACE}" delete vpcs.ec2.services.k8s.aws "${NETWORK_NAME}-vpc" --ignore-not-found || true

	info "Waiting for VPC deletion (timeout=${WAIT_TIMEOUT_NETWORK})"
	k -n "${NAMESPACE}" wait --for=delete --timeout="${WAIT_TIMEOUT_NETWORK}" vpcs.ec2.services.k8s.aws/"${NETWORK_NAME}-vpc" || true
fi

info "Verifying AWS-side EKS deletion (timeout=${WAIT_TIMEOUT_AWS})"
wait_for_aws_eks_disappearance "${WAIT_TIMEOUT_AWS}" || true

info "Remaining ACK resources (may take time to finalize)"
k -n "${NAMESPACE}" get \
	clusters.eks.services.k8s.aws,roles.iam.services.k8s.aws,instanceprofiles.iam.services.k8s.aws,\
	vpcs.ec2.services.k8s.aws,subnets.ec2.services.k8s.aws,natgateways.ec2.services.k8s.aws,securitygroups.ec2.services.k8s.aws \
	-o wide || true

info "If resources remain stuck, see break-glass docs: docs/eks/fargate/break-glass.md"
info "Done"
