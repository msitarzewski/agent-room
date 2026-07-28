#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'

public_url=""
cookie_jar=""
allowed_project=""
denied_project=""
while (($#)); do
  case "$1" in
    --public-url) public_url="${2:?}"; shift 2 ;;
    --cookie-jar) cookie_jar="${2:?}"; shift 2 ;;
    --allowed-project) allowed_project="${2:?}"; shift 2 ;;
    --denied-project) denied_project="${2:?}"; shift 2 ;;
    -h|--help)
      printf 'Usage: session-boundary-regression.sh --public-url HTTPS_ORIGIN --cookie-jar PATH --allowed-project ID --denied-project ID\n'
      exit 0
      ;;
    *) printf 'Unknown argument: %s\n' "$1" >&2; exit 2 ;;
  esac
done

[[ "${public_url}" =~ ^https://[^/]+$ ]] || {
  printf 'Public URL must be an HTTPS origin\n' >&2
  exit 2
}
[[ -r "${cookie_jar}" && -n "${allowed_project}" && -n "${denied_project}" &&
   "${allowed_project}" != "${denied_project}" ]] || {
  printf 'A cookie jar and two distinct project IDs are required\n' >&2
  exit 2
}
mode="$(stat -c '%a' "${cookie_jar}" 2>/dev/null || stat -f '%Lp' "${cookie_jar}")"
(( (8#${mode} & 8#077) == 0 )) || {
  printf 'Cookie jar must not be accessible to group or other\n' >&2
  exit 1
}
command -v curl >/dev/null 2>&1 || { printf 'curl is required\n' >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { printf 'jq is required\n' >&2; exit 1; }

temporary="$(mktemp -d)"
trap 'rm -rf -- "${temporary}"' EXIT
curl_common=(--silent --show-error --max-time 10 --cookie "${cookie_jar}")
allowed_query="$(printf '%s' "${allowed_project}" | jq -sRr @uri)"
denied_query="$(printf '%s' "${denied_project}" | jq -sRr @uri)"

curl --fail "${curl_common[@]}" --output "${temporary}/session.json" \
  "${public_url}/api/v1/auth/session"
csrf="$(jq -er '.csrf_token | select(type == "string" and length >= 32)' \
  "${temporary}/session.json")"

allowed_status="$(curl "${curl_common[@]}" --output /dev/null --write-out '%{http_code}' \
  "${public_url}/api/v1/tasks?project_id=${allowed_query}")"
[[ "${allowed_status}" == "200" ]] || {
  printf 'Authorized project returned %s, expected 200\n' "${allowed_status}" >&2
  exit 1
}

denied_status="$(curl "${curl_common[@]}" --output /dev/null --write-out '%{http_code}' \
  "${public_url}/api/v1/tasks?project_id=${denied_query}")"
[[ "${denied_status}" == "403" ]] || {
  printf 'Unauthorized project returned %s, expected 403\n' "${denied_status}" >&2
  exit 1
}

missing_csrf_status="$(curl "${curl_common[@]}" --output /dev/null --write-out '%{http_code}' \
  --request POST --header "Origin: ${public_url}" \
  "${public_url}/api/v1/auth/logout")"
[[ "${missing_csrf_status}" == "403" ]] || {
  printf 'Mutation without CSRF returned %s, expected 403\n' "${missing_csrf_status}" >&2
  exit 1
}

foreign_origin_status="$(curl "${curl_common[@]}" --output /dev/null --write-out '%{http_code}' \
  --request POST --header 'Origin: https://attacker.invalid' \
  --header "X-CSRF-Token: ${csrf}" \
  "${public_url}/api/v1/auth/logout")"
[[ "${foreign_origin_status}" == "403" ]] || {
  printf 'Foreign-origin mutation returned %s, expected 403\n' "${foreign_origin_status}" >&2
  exit 1
}

websocket_status="$(curl "${curl_common[@]}" --http1.1 \
  --header 'Connection: Upgrade' \
  --header 'Upgrade: websocket' \
  --header 'Sec-WebSocket-Version: 13' \
  --header 'Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==' \
  --header 'Origin: https://attacker.invalid' \
  --output /dev/null --write-out '%{http_code}' \
  "${public_url}/api/v1/stream?project_id=${allowed_query}")"
[[ "${websocket_status}" == "403" ]] || {
  printf 'Authenticated foreign-origin WebSocket returned %s, expected 403\n' \
    "${websocket_status}" >&2
  exit 1
}

curl --fail "${curl_common[@]}" --output /dev/null "${public_url}/api/v1/auth/session"
printf 'Project isolation, CSRF, Origin, and authenticated WebSocket boundaries passed\n'
