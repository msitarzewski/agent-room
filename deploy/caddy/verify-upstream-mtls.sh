#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'

usage() {
  printf 'Usage: verify-upstream-mtls.sh --url HTTPS_URL --ca PATH --client-cert PATH --client-key PATH [--untrusted-cert PATH --untrusted-key PATH]\n' >&2
}

url=""
ca=""
client_cert=""
client_key=""
untrusted_cert=""
untrusted_key=""

while (($#)); do
  case "$1" in
    --url) url="${2:?}"; shift 2 ;;
    --ca) ca="${2:?}"; shift 2 ;;
    --client-cert) client_cert="${2:?}"; shift 2 ;;
    --client-key) client_key="${2:?}"; shift 2 ;;
    --untrusted-cert) untrusted_cert="${2:?}"; shift 2 ;;
    --untrusted-key) untrusted_key="${2:?}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) usage; exit 2 ;;
  esac
done

[[ "${url}" =~ ^https://[^/]+(:[0-9]+)?/healthz$ ]] || {
  printf 'The URL must be an HTTPS /healthz endpoint\n' >&2
  exit 2
}
for path in "${ca}" "${client_cert}" "${client_key}"; do
  [[ -f "${path}" ]] || { printf 'Required certificate material not found: %s\n' "${path}" >&2; exit 1; }
done
if [[ -n "${untrusted_cert}" || -n "${untrusted_key}" ]]; then
  [[ -f "${untrusted_cert}" && -f "${untrusted_key}" ]] || {
    printf 'Both untrusted certificate and key are required\n' >&2
    exit 2
  }
fi
command -v curl >/dev/null 2>&1 || { printf 'curl is required\n' >&2; exit 1; }

curl_flags=(--fail --silent --show-error --connect-timeout 3 --max-time 10 --cacert "${ca}")

if curl "${curl_flags[@]}" "${url}" >/dev/null 2>&1; then
  printf 'FAIL: upstream accepted a client without a certificate\n' >&2
  exit 1
fi

curl "${curl_flags[@]}" --cert "${client_cert}" --key "${client_key}" "${url}" >/dev/null

if [[ -n "${untrusted_cert}" ]] &&
   curl "${curl_flags[@]}" --cert "${untrusted_cert}" --key "${untrusted_key}" "${url}" >/dev/null 2>&1; then
  printf 'FAIL: upstream accepted an untrusted client certificate\n' >&2
  exit 1
fi

printf 'PASS: upstream requires the trusted mTLS client identity\n'
