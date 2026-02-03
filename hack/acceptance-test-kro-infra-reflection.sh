#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="${NAMESPACE:-default}"
CLUSTER_NAME="${CLUSTER_NAME:-demo-cluster}"
KRO_VERSION="${KRO_VERSION:-0.7.1}"
IMG="${IMG:-example.com/kany8s:acceptance-kro-infra}"

RGD_NAME="demo-infra.kro.run"
RGD_INSTANCE_CRD="demoinfrastructures.kro.run"

echo "error: kro infra reflection acceptance script is not implemented yet" >&2
echo "see docs/issues/kany8cluster-at-todo.md" >&2
exit 1
