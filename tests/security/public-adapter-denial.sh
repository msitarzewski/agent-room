#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'

public_url=""
token_file=""
project_id=""
while (($#)); do
  case "$1" in
    --public-url) public_url="${2:?}"; shift 2 ;;
    --token-file) token_file="${2:?}"; shift 2 ;;
    --project-id) project_id="${2:?}"; shift 2 ;;
    -h|--help)
      printf 'Usage: public-adapter-denial.sh --public-url HTTPS_ORIGIN --token-file PATH --project-id ID\n'
      exit 0
      ;;
    *) printf 'Unknown argument: %s\n' "$1" >&2; exit 2 ;;
  esac
done

[[ "${public_url}" =~ ^https://[^/]+$ ]] || {
  printf 'Public URL must be an HTTPS origin\n' >&2
  exit 2
}
[[ -r "${token_file}" && -n "${project_id}" ]] || {
  printf 'A readable token file and project ID are required\n' >&2
  exit 2
}
mode="$(stat -c '%a' "${token_file}" 2>/dev/null || stat -f '%Lp' "${token_file}")"
(( (8#${mode} & 8#077) == 0 )) || {
  printf 'Token file must not be accessible to group or other\n' >&2
  exit 1
}
token="$(tr -d '\r\n' <"${token_file}")"
[[ -n "${token}" ]] || { printf 'Token file is empty\n' >&2; exit 1; }
temporary="$(mktemp -d)"
trap 'rm -rf -- "${temporary}"' EXIT
authorization_header="${temporary}/authorization.header"
install -m 0600 /dev/null "${authorization_header}"
printf 'Authorization: Bearer %s\n' "${token}" >"${authorization_header}"
unset token

for path in /api/v1/ingest /api/v1/mcp /api/v1/adapters/hermes/callback; do
  status="$(curl --silent --show-error --output /dev/null --write-out '%{http_code}' \
    --max-time 10 \
    --header "@${authorization_header}" \
    --header 'Content-Type: application/json' \
    --data '{}' \
    "${public_url}${path}?project_id=$(printf '%s' "${project_id}" | jq -sRr @uri)")"
  [[ "${status}" == "404" ]] || {
    printf 'Public adapter path %s returned %s, expected 404\n' "${path}" "${status}" >&2
    exit 1
  }
done

printf 'Public ingress denied all adapter paths even with a service token\n'
