#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="${NAMESPACE:-default}"
CLUSTER_NAME="${CLUSTER_NAME:-}"
NETWORK_NAME="${NETWORK_NAME:-}"

KUBECTL_CONTEXT="${KUBECTL_CONTEXT:-}"

DELETE_NETWORK="${DELETE_NETWORK:-true}"
WAIT_TIMEOUT_EKS="${WAIT_TIMEOUT_EKS:-40m}"
WAIT_TIMEOUT_NETWORK="${WAIT_TIMEOUT_NETWORK:-30m}"

CONFIRM="${CONFIRM:-false}"

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

if [[ "${CONFIRM}" != "true" ]]; then
	echo "This script deletes Kubernetes (and optionally AWS via ACK) resources." >&2
	echo "Set CONFIRM=true to proceed." >&2
	echo >&2
	echo "Planned actions:" >&2
	echo "- kubectl -n ${NAMESPACE} delete cluster.cluster.x-k8s.io/${CLUSTER_NAME}" >&2
	if [[ "${DELETE_NETWORK}" == "true" ]]; then
		echo "- kubectl -n ${NAMESPACE} delete ACK EC2 network resources for ${NETWORK_NAME}-*" >&2
	fi
	exit 2
fi

info "Current resources (bootstrapper-managed)"
k -n "${NAMESPACE}" get \
	openidconnectproviders.iam.services.k8s.aws,roles.iam.services.k8s.aws,policies.iam.services.k8s.aws,instanceprofiles.iam.services.k8s.aws,\
	accessentries.eks.services.k8s.aws,fargateprofiles.eks.services.k8s.aws,\
	ocirepositories.source.toolkit.fluxcd.io,helmreleases.helm.toolkit.fluxcd.io,\
	configmaps,clusterresourcesets.addons.cluster.x-k8s.io \
	-l cluster.x-k8s.io/cluster-name="${CLUSTER_NAME}" \
	-o wide || true

info "Deleting CAPI Cluster ${NAMESPACE}/${CLUSTER_NAME}"
k -n "${NAMESPACE}" delete cluster.cluster.x-k8s.io "${CLUSTER_NAME}" --ignore-not-found

info "Waiting for CAPI Cluster deletion (timeout=${WAIT_TIMEOUT_EKS})"
k -n "${NAMESPACE}" wait --for=delete --timeout="${WAIT_TIMEOUT_EKS}" cluster.cluster.x-k8s.io/"${CLUSTER_NAME}" || true

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

info "Remaining ACK resources (may take time to finalize)"
k -n "${NAMESPACE}" get \
	clusters.eks.services.k8s.aws,roles.iam.services.k8s.aws,instanceprofiles.iam.services.k8s.aws,\
	vpcs.ec2.services.k8s.aws,subnets.ec2.services.k8s.aws,natgateways.ec2.services.k8s.aws,securitygroups.ec2.services.k8s.aws \
	-o wide || true

info "Done"
