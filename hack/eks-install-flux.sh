#!/usr/bin/env bash
set -euo pipefail

: "${FLUX_VERSION:=v2.4.0}"
: "${ROLLOUT_TIMEOUT:=300s}"

manifest_url="https://github.com/fluxcd/flux2/releases/download/${FLUX_VERSION}/install.yaml"

echo "==> Installing Flux ${FLUX_VERSION}"
kubectl apply -f "${manifest_url}"

echo "==> Waiting for Flux controllers"
kubectl -n flux-system rollout status deploy/source-controller --timeout="${ROLLOUT_TIMEOUT}"
kubectl -n flux-system rollout status deploy/helm-controller --timeout="${ROLLOUT_TIMEOUT}"

echo "==> Flux install complete"
echo "    manifest=${manifest_url}"
