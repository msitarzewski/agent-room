#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'

config="${1:-/etc/caddy/Caddyfile}"
command -v caddy >/dev/null 2>&1 || { printf 'caddy is required\n' >&2; exit 1; }
[[ -f "${config}" ]] || { printf 'Caddy config not found: %s\n' "${config}" >&2; exit 1; }
grep -q 'tls_client_auth' "${config}" || { printf 'mTLS client authentication is absent\n' >&2; exit 1; }
grep -q 'tls_trust_pool' "${config}" || { printf 'private upstream trust pool is absent\n' >&2; exit 1; }
! grep -q 'tls_insecure_skip_verify' "${config}" || {
  printf 'tls_insecure_skip_verify is forbidden\n' >&2
  exit 1
}
grep -q 'handle @allowed' "${config}" || { printf 'public route allowlist is absent\n' >&2; exit 1; }
grep -q 'handle @private_adapters' "${config}" || { printf 'private adapter denial is absent\n' >&2; exit 1; }
grep -q '/api/v1/ingest /api/v1/mcp' "${config}" || {
  printf 'ingest and MCP are not explicitly denied at public ingress\n' >&2
  exit 1
}
grep -q 'respond "Not Found" 404' "${config}" || { printf 'default-deny route is absent\n' >&2; exit 1; }
caddy fmt --diff "${config}"
caddy validate --config "${config}" --adapter caddyfile
