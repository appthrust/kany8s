#!/usr/bin/env bash
set -euo pipefail

timestamp="${TIMESTAMP:-$(date +%Y%m%d%H%M%S)}"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

base_artifacts_dir="${ARTIFACTS_DIR:-/tmp/kany8s-acceptance-all-${timestamp}}"

echo "==> Acceptance run-all"
echo "    TIMESTAMP=${timestamp}"
echo "    ARTIFACTS_DIR=${base_artifacts_dir}"

mkdir -p "${base_artifacts_dir}"

echo "==> Running: kro acceptance (kro infra reflection)"
TIMESTAMP="${timestamp}" \
ARTIFACTS_DIR="${base_artifacts_dir}/acceptance-kro-infra-reflection" \
KIND_CLUSTER_NAME="kany8s-acc-infra-${timestamp}" \
"${repo_root}/test/acceptance_test/run-acceptance-kro-infra-reflection.sh"

echo "==> OK: acceptance run-all completed"
