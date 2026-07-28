#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'

adapter_url="http://127.0.0.1:9091"
privileged_token_file=""
restricted_token_file=""
project_id=""
while (($#)); do
  case "$1" in
    --adapter-url) adapter_url="${2:?}"; shift 2 ;;
    --privileged-token-file) privileged_token_file="${2:?}"; shift 2 ;;
    --restricted-token-file) restricted_token_file="${2:?}"; shift 2 ;;
    --project-id) project_id="${2:?}"; shift 2 ;;
    -h|--help)
      printf '%s\n' \
        'Usage: mcp-session-binding-regression.sh --privileged-token-file TASK_READ_TOKEN' \
        '  --restricted-token-file NO_TASK_READ_TOKEN --project-id ID' \
        '  [--adapter-url LOOPBACK_URL]'
      exit 0
      ;;
    *) printf 'Unknown argument: %s\n' "$1" >&2; exit 2 ;;
  esac
done

[[ "${adapter_url}" =~ ^http://(127\.0\.0\.1|\[::1\]|localhost):[0-9]+$ ]] || {
  printf 'Adapter test URL must be loopback HTTP\n' >&2
  exit 2
}
[[ -r "${privileged_token_file}" && -r "${restricted_token_file}" &&
   -n "${project_id}" && "${privileged_token_file}" != "${restricted_token_file}" ]] || {
  printf 'Two distinct readable token files and a project ID are required\n' >&2
  exit 2
}
command -v jq >/dev/null 2>&1 || { printf 'jq is required\n' >&2; exit 1; }

temporary="$(mktemp -d)"
trap 'rm -rf -- "${temporary}"' EXIT
make_header_file() {
  local source_file="$1"
  local destination="$2"
  local mode
  local token
  mode="$(stat -c '%a' "${source_file}" 2>/dev/null || stat -f '%Lp' "${source_file}")"
  (( (8#${mode} & 8#077) == 0 )) || {
    printf 'Token file must not be accessible to group or other: %s\n' "${source_file}" >&2
    return 1
  }
  token="$(tr -d '\r\n' <"${source_file}")"
  [[ -n "${token}" ]] || { printf 'Token file is empty: %s\n' "${source_file}" >&2; return 1; }
  install -m 0600 /dev/null "${destination}"
  printf 'Authorization: Bearer %s\n' "${token}" >"${destination}"
}
make_header_file "${privileged_token_file}" "${temporary}/privileged.header"
make_header_file "${restricted_token_file}" "${temporary}/restricted.header"

endpoint="${adapter_url}/api/v1/mcp?project_id=$(printf '%s' "${project_id}" | jq -sRr @uri)"
protocol_headers=(
  --header 'Content-Type: application/json'
  --header 'Accept: application/json, text/event-stream'
)
initialize_status="$(curl --silent --show-error --max-time 10 \
  --header "@${temporary}/privileged.header" "${protocol_headers[@]}" \
  --dump-header "${temporary}/initialize.headers" \
  --output "${temporary}/initialize.body" --write-out '%{http_code}' \
  --data '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"agentroom-security","version":"1"}}}' \
  "${endpoint}")"
[[ "${initialize_status}" == "200" ]] || {
  printf 'Privileged MCP initialization returned %s\n' "${initialize_status}" >&2
  exit 1
}

session_id="$(awk 'BEGIN{IGNORECASE=1} /^Mcp-Session-Id:/ {sub(/\r$/,"",$2); print $2}' \
  "${temporary}/initialize.headers")"
session_header=()
if [[ -n "${session_id}" ]]; then
  session_header=(--header "Mcp-Session-Id: ${session_id}")
fi
call_status="$(curl --silent --show-error --max-time 10 \
  --header "@${temporary}/restricted.header" "${protocol_headers[@]}" \
  "${session_header[@]}" --output "${temporary}/call.body" --write-out '%{http_code}' \
  --data '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_tasks","arguments":{"limit":1}}}' \
  "${endpoint}")"

if [[ "${call_status}" == "200" ]]; then
  compact="$(tr -d '\r\n' <"${temporary}/call.body")"
  if [[ "${compact}" == *'"result"'* && "${compact}" != *'"isError":true'* &&
       "${compact}" != *'capability_denied'* && "${compact}" != *'permission denied'* ]]; then
    printf 'Restricted token inherited task-read authority from an MCP session\n' >&2
    exit 1
  fi
elif [[ "${call_status}" != "403" && "${call_status}" != "404" ]]; then
  printf 'Restricted session call returned unexpected status %s\n' "${call_status}" >&2
  exit 1
fi

printf 'MCP session did not transfer task-read authority across service tokens\n'
