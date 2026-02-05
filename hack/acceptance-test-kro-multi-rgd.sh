#!/usr/bin/env bash
set -euo pipefail

# Legacy alias. Prefer: hack/acceptance-test-kro-reflection-multi-rgd.sh
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec "${script_dir}/acceptance-test-kro-reflection-multi-rgd.sh" "$@"
