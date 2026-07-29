#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'

config="${1:-/etc/caddy/Caddyfile}"
command -v caddy >/dev/null 2>&1 || { printf 'caddy is required\n' >&2; exit 1; }
[[ -f "${config}" ]] || { printf 'Caddy config not found: %s\n' "${config}" >&2; exit 1; }
grep -Eq 'reverse_proxy[[:space:]]+(http://)?127\.0\.0\.1:8443([[:space:]]|$)' "${config}" || {
  printf 'loopback HTTP upstream is absent\n' >&2
  exit 1
}
if grep -Eq 'reverse_proxy[[:space:]]+https://127\.0\.0\.1:8443' "${config}"; then
  printf 'obsolete upstream TLS configuration is present\n' >&2
  exit 1
fi
grep -Eq '@private.*path|@private[[:space:]]+path' "${config}" ||
  { printf 'private route matcher is absent\n' >&2; exit 1; }
for private_path in /api/v1/ingest /api/v1/mcp /api/v1/adapters; do
  grep -q "${private_path}" "${config}" ||
    { printf 'private route denial is missing %s\n' "${private_path}" >&2; exit 1; }
done
grep -Eq 'respond[[:space:]]+@private([[:space:]]+"Not Found")?[[:space:]]+404|handle[[:space:]]+@private' "${config}" ||
  { printf 'private route denial is absent\n' >&2; exit 1; }
if ! caddy fmt --diff "${config}" >/dev/null; then
  printf 'Caddyfile is not formatted; run: caddy fmt --overwrite %s\n' "${config}" >&2
  exit 1
fi
caddy validate --config "${config}" --adapter caddyfile
