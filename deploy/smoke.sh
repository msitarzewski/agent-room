#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'

private_url="http://127.0.0.1:9090"
public_url=""
timeout_seconds=10

while (($#)); do
  case "$1" in
    --private-url) private_url="${2:?}"; shift 2 ;;
    --public-url) public_url="${2:?}"; shift 2 ;;
    --timeout) timeout_seconds="${2:?}"; shift 2 ;;
    -h|--help)
      printf 'Usage: smoke.sh --public-url HTTPS_URL [--private-url URL] [--timeout SECONDS]\n'
      exit 0
      ;;
    *) printf 'Unknown argument: %s\n' "$1" >&2; exit 2 ;;
  esac
done

[[ "${private_url}" =~ ^http://127\.0\.0\.1:[0-9]+$ ]] || {
  printf 'Private admin smoke target must be loopback HTTP\n' >&2
  exit 1
}
[[ "${public_url}" =~ ^https://[^/]+$ ]] || {
  printf 'A public origin such as https://agentroom.example is required\n' >&2
  exit 1
}
[[ "${timeout_seconds}" =~ ^[1-9][0-9]*$ ]] || { printf 'Invalid timeout\n' >&2; exit 1; }
command -v curl >/dev/null 2>&1 || { printf 'curl is required\n' >&2; exit 1; }

curl_common=(--fail --silent --show-error --max-time "${timeout_seconds}")
curl "${curl_common[@]}" "${private_url}/livez" >/dev/null
curl "${curl_common[@]}" "${private_url}/readyz" >/dev/null
curl "${curl_common[@]}" "${public_url}/healthz" >/dev/null

headers="$(mktemp)"
trap 'rm -f -- "${headers}"' EXIT
curl "${curl_common[@]}" --dump-header "${headers}" --output /dev/null "${public_url}/"
for required_header in \
  'strict-transport-security:' \
  'content-security-policy:' \
  'x-content-type-options:' \
  'referrer-policy:'; do
  grep -qi "^${required_header}" "${headers}" || {
    printf 'Required public security header is absent: %s\n' "${required_header}" >&2
    exit 1
  }
done

for private_path in \
  /livez \
  /readyz \
  /metrics \
  /debug/pprof/ \
  /mcp \
  /admin/migrations \
  /api/v1/ingest \
  /api/v1/mcp \
  /api/v1/adapters/hermes/callback; do
  status="$(curl --silent --output /dev/null --write-out '%{http_code}' \
    --max-time "${timeout_seconds}" "${public_url}${private_path}")"
  [[ "${status}" == "404" ]] || {
    printf 'Private route %s returned %s through the public origin, expected 404\n' \
      "${private_path}" "${status}" >&2
    exit 1
  }
done

printf 'Agent Room private and public smoke checks passed\n'
