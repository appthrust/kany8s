#!/usr/bin/env bash
set -euo pipefail

# Legacy alias. Prefer: test/acceptance_test/run-acceptance-kro-reflection-multi-rgd.sh
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec "${script_dir}/run-acceptance-kro-reflection-multi-rgd.sh" "$@"
