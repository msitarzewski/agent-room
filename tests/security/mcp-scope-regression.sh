#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'

adapter_url="http://127.0.0.1:9091"
token_file=""
project_id=""
while (($#)); do
  case "$1" in
    --adapter-url) adapter_url="${2:?}"; shift 2 ;;
    --token-file) token_file="${2:?}"; shift 2 ;;
    --project-id) project_id="${2:?}"; shift 2 ;;
    -h|--help)
      printf 'Usage: mcp-scope-regression.sh --token-file INGEST_ONLY_TOKEN --project-id ID [--adapter-url LOOPBACK_URL]\n'
      exit 0
      ;;
    *) printf 'Unknown argument: %s\n' "$1" >&2; exit 2 ;;
  esac
done

[[ "${adapter_url}" =~ ^http://(127\.0\.0\.1|\[::1\]|localhost):[0-9]+$ ]] || {
  printf 'Adapter test URL must be loopback HTTP\n' >&2
  exit 2
}
[[ -r "${token_file}" && -n "${project_id}" ]] || {
  printf 'A readable ingest-only token file and project ID are required\n' >&2
  exit 2
}
mode="$(stat -c '%a' "${token_file}" 2>/dev/null || stat -f '%Lp' "${token_file}")"
(( (8#${mode} & 8#077) == 0 )) || {
  printf 'Token file must not be accessible to group or other\n' >&2
  exit 1
}
token="$(tr -d '\r\n' <"${token_file}")"
[[ -n "${token}" ]] || { printf 'Token file is empty\n' >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { printf 'jq is required\n' >&2; exit 1; }

endpoint="${adapter_url}/api/v1/mcp?project_id=$(printf '%s' "${project_id}" | jq -sRr @uri)"
temporary="$(mktemp -d)"
trap 'rm -rf -- "${temporary}"' EXIT
authorization_header="${temporary}/authorization.header"
install -m 0600 /dev/null "${authorization_header}"
printf 'Authorization: Bearer %s\n' "${token}" >"${authorization_header}"
unset token
common_headers=(
  --header "@${authorization_header}"
  --header 'Content-Type: application/json'
  --header 'Accept: application/json, text/event-stream'
)

curl --fail --silent --show-error --max-time 10 \
  "${common_headers[@]}" \
  --dump-header "${temporary}/initialize.headers" \
  --output "${temporary}/initialize.body" \
  --data '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"agentroom-security","version":"1"}}}' \
  "${endpoint}"

session_id="$(awk 'BEGIN{IGNORECASE=1} /^Mcp-Session-Id:/ {sub(/\r$/,"",$2); print $2}' \
  "${temporary}/initialize.headers")"
session_header=()
if [[ -n "${session_id}" ]]; then
  session_header=(--header "Mcp-Session-Id: ${session_id}")
fi

curl --fail --silent --show-error --max-time 10 \
  "${common_headers[@]}" "${session_header[@]}" \
  --output "${temporary}/call.body" \
  --data '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_tasks","arguments":{"limit":1}}}' \
  "${endpoint}"

compact="$(tr -d '\r\n' <"${temporary}/call.body")"
if [[ "${compact}" == *'"result"'* && "${compact}" != *'"isError":true'* &&
     "${compact}" != *'capability_denied'* && "${compact}" != *'permission denied'* ]]; then
  printf 'Ingest-only service token successfully called MCP list_tasks\n' >&2
  exit 1
fi

printf 'Ingest-only service token was denied MCP list_tasks\n'
