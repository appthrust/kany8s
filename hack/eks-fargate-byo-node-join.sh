#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

: "${NAMESPACE:=default}"
: "${CLUSTER_NAME:?set CLUSTER_NAME}"
: "${KIND_CLUSTER_NAME:=kany8s-eks}"
: "${INSTALL_FLUX:=true}"
: "${FLUX_VERSION:=v2.4.0}"
: "${WAIT_TIMEOUT_SECONDS:=2400}"
: "${WAIT_INTERVAL_SECONDS:=10}"
: "${ALLOW_UNMANAGED_TAKEOVER:=false}"

wait_for() {
  local description="$1"
  local cmd="$2"
  local start now

  start="$(date +%s)"
  while true; do
    if bash -c "${cmd}" >/dev/null 2>&1; then
      echo "==> OK: ${description}"
      return 0
    fi
    now="$(date +%s)"
    if (( now - start >= WAIT_TIMEOUT_SECONDS )); then
      echo "ERROR: timeout waiting for ${description}" >&2
      echo "       command: ${cmd}" >&2
      return 1
    fi
    sleep "${WAIT_INTERVAL_SECONDS}"
  done
}

echo "==> EKS BYO + plugins + opt-in node join"
echo "    namespace=${NAMESPACE}"
echo "    cluster=${CLUSTER_NAME}"
echo "    kind=${KIND_CLUSTER_NAME}"

if [[ "${INSTALL_FLUX}" == "true" ]]; then
  echo "==> Installing pinned Flux (${FLUX_VERSION})"
  FLUX_VERSION="${FLUX_VERSION}" "${repo_root}/hack/eks-install-flux.sh"
fi

echo "==> Deploying EKS plugins (kind overlay)"
kubectl apply -k "${repo_root}/config/overlays/eks-plugin/kind"
kubectl apply -k "${repo_root}/config/overlays/eks-karpenter-bootstrapper/kind"
kubectl -n ack-system rollout status deploy/eks-kubeconfig-rotator --timeout=300s
kubectl -n ack-system rollout status deploy/eks-karpenter-bootstrapper --timeout=300s

echo "==> Enabling plugin opt-in on Cluster"
kubectl -n "${NAMESPACE}" annotate cluster "${CLUSTER_NAME}" eks.kany8s.io/kubeconfig-rotator=enabled --overwrite
kubectl -n "${NAMESPACE}" label cluster "${CLUSTER_NAME}" eks.kany8s.io/karpenter=enabled --overwrite
if [[ "${ALLOW_UNMANAGED_TAKEOVER}" == "true" ]]; then
  kubectl -n "${NAMESPACE}" annotate cluster "${CLUSTER_NAME}" eks.kany8s.io/allow-unmanaged-takeover=enabled --overwrite
fi

wait_for "kubeconfig exec Secret" \
  "kubectl -n \"${NAMESPACE}\" get secret \"${CLUSTER_NAME}-kubeconfig-exec\""

wait_for "bootstrapper FargateProfile resources" \
  "kubectl -n \"${NAMESPACE}\" get fargateprofiles.eks.services.k8s.aws \"${CLUSTER_NAME}-fargate-coredns\" && kubectl -n \"${NAMESPACE}\" get fargateprofiles.eks.services.k8s.aws \"${CLUSTER_NAME}-fargate-karpenter\""

wait_for "Flux HelmRelease for Karpenter" \
  "kubectl -n \"${NAMESPACE}\" get helmreleases.helm.toolkit.fluxcd.io \"${CLUSTER_NAME}-karpenter\""

tmp_kubeconfig="${TMPDIR:-/tmp}/${CLUSTER_NAME}-kubeconfig-exec"
kubectl -n "${NAMESPACE}" get secret "${CLUSTER_NAME}-kubeconfig-exec" -o jsonpath='{.data.value}' | base64 -d > "${tmp_kubeconfig}"
chmod 0600 "${tmp_kubeconfig}"

wait_for "at least one joined workload node" \
  "KUBECONFIG=\"${tmp_kubeconfig}\" kubectl get nodes --no-headers | grep -q ."

echo "==> SUCCESS: node join confirmed"
echo "    kubeconfig=${tmp_kubeconfig}"
