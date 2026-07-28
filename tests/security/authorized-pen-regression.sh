#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
target="${1:-}"

[[ "${I_AM_AUTHORIZED_TO_SCAN:-}" == "YES" ]] || {
  printf 'Explicit authorization acknowledgement is required\n' >&2
  exit 1
}
"${script_dir}/http-boundary.sh" "${target}"
"${script_dir}/zap/run.sh" "${target}"
printf 'Automated authorized penetration regressions passed; manual testing remains required\n'
