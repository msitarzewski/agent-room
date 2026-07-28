#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
target="${1:-}"

[[ "${I_AM_AUTHORIZED_TO_SCAN:-}" == "YES" ]] || {
  printf 'Set I_AM_AUTHORIZED_TO_SCAN=YES only for an explicitly authorized staging target\n' >&2
  exit 1
}
[[ "${target}" =~ ^https://[^/]+$ ]] || {
  printf 'Usage: run.sh https://authorized-staging-origin\n' >&2
  exit 2
}
case "${target}" in
  *localhost*|*127.0.0.1*|*.test|*.invalid|*staging*) ;;
  *)
    [[ "${ALLOW_NON_STAGING_TARGET:-}" == "YES" ]] || {
      printf 'Target does not look like staging; refusing active scan\n' >&2
      exit 1
    }
    ;;
esac
command -v docker >/dev/null 2>&1 || { printf 'docker is required\n' >&2; exit 1; }

work="$(mktemp -d)"
trap 'rm -rf -- "${work}"' EXIT
mkdir -p "${work}/results"
escaped_target="$(printf '%s' "${target}" | sed 's/[&|]/\\&/g')"
sed "s|__TARGET_URL__|${escaped_target}|g" \
  "${script_dir}/automation.yaml.template" >"${work}/automation.yaml"

docker run --rm --network host \
  -v "${work}:/zap/wrk:rw" \
  ghcr.io/zaproxy/zaproxy:2.17.0@sha256:8d387b1a63e3425beef4846e39719f5af2a787753af2d8b6558c6257d7a577a2 \
  zap.sh -cmd -autorun /zap/wrk/automation.yaml
[[ -s "${work}/results/zap-report.json" ]] || {
  printf 'ZAP completed without producing its required report\n' >&2
  exit 1
}
install -m 0600 "${work}/results/zap-report.json" \
  "${ZAP_REPORT_PATH:-${script_dir}/zap-report.json}"
