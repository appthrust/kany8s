#!/usr/bin/env bash
set -euo pipefail

# Legacy alias. Prefer: hack/acceptance-test-capd-kubeadm.sh
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec "${script_dir}/acceptance-test-capd-kubeadm.sh" "$@"
