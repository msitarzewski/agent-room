#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'

origin="${1:-}"
[[ "${origin}" =~ ^https://[^/]+$ ]] || {
  printf 'Usage: http-boundary.sh https://staging-origin\n' >&2
  exit 2
}
command -v curl >/dev/null 2>&1 || { printf 'curl is required\n' >&2; exit 1; }

timeout=10
curl_flags=(--silent --show-error --max-time "${timeout}")
status="$(curl "${curl_flags[@]}" --output /dev/null --write-out '%{http_code}' "${origin}/healthz")"
[[ "${status}" == "200" ]] || { printf '/healthz returned %s\n' "${status}" >&2; exit 1; }

for path in /livez /readyz /metrics /debug/pprof/ /mcp /admin/migrations; do
  status="$(curl "${curl_flags[@]}" --output /dev/null --write-out '%{http_code}' "${origin}${path}")"
  [[ "${status}" == "404" ]] || {
    printf 'Private route %s was externally distinguishable: %s\n' "${path}" "${status}" >&2
    exit 1
  }
done

for spoof in \
  'X-Remote-User: administrator' \
  'X-Forwarded-User: administrator' \
  'X-Forwarded-For: 127.0.0.1' \
  'X-Forwarded-Proto: http'; do
  status="$(curl "${curl_flags[@]}" -H "${spoof}" --output /dev/null \
    --write-out '%{http_code}' "${origin}/api/v1/agents")"
  [[ "${status}" != 200 && "${status}" != 201 && "${status}" != 204 ]] || {
    printf 'Forged proxy header produced an authenticated response: %s\n' "${spoof}" >&2
    exit 1
  }
done

ws_status="$(curl "${curl_flags[@]}" --http1.1 \
  -H 'Connection: Upgrade' \
  -H 'Upgrade: websocket' \
  -H 'Sec-WebSocket-Version: 13' \
  -H 'Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==' \
  -H 'Origin: https://attacker.invalid' \
  --output /dev/null --write-out '%{http_code}' "${origin}/ws")"
[[ "${ws_status}" != "101" ]] || {
  printf 'Foreign-origin unauthenticated WebSocket upgrade succeeded\n' >&2
  exit 1
}

headers="$(mktemp)"
trap 'rm -f -- "${headers}"' EXIT
curl "${curl_flags[@]}" --dump-header "${headers}" --output /dev/null "${origin}/"
for header in strict-transport-security content-security-policy x-content-type-options referrer-policy; do
  grep -qi "^${header}:" "${headers}" || {
    printf 'Missing security header: %s\n' "${header}" >&2
    exit 1
  }
done
grep -Eqi '^content-security-policy:.*frame-ancestors[[:space:]]+'\''none'\''' "${headers}" || {
  printf 'CSP does not deny framing\n' >&2
  exit 1
}
for directive in \
  "base-uri 'none'" \
  "form-action 'self'" \
  "object-src 'none'" \
  "script-src 'self'" \
  "style-src 'self'" \
  "upgrade-insecure-requests"; do
  grep -Fqi "${directive}" "${headers}" || {
    printf 'CSP is missing directive: %s\n' "${directive}" >&2
    exit 1
  }
done

curl "${curl_flags[@]}" --dump-header "${headers}" --output /dev/null \
  "${origin}/api/v1/agents?project_id=unauthenticated-cache-check"
grep -Eqi '^cache-control:[[:space:]]*.*no-store' "${headers}" || {
  printf 'API response is missing Cache-Control: no-store\n' >&2
  exit 1
}

printf 'Unauthenticated HTTP and WebSocket boundary regressions passed\n'
