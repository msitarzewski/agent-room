#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
cd "${repo_root}"

for tool in gitleaks semgrep gosec govulncheck staticcheck go npm; do
  command -v "${tool}" >/dev/null 2>&1 || {
    printf 'Required security tool unavailable: %s\n' "${tool}" >&2
    exit 1
  }
done

gitleaks git --redact --config .gitleaks.toml .
gitleaks dir --redact --config .gitleaks.toml .
semgrep scan --error --config .semgrep.yml .
# G124 is covered by TestCookieSecurityAttributes: production always sets
# Secure while loopback-only development deliberately does not. G204 is covered
# by the runtime boundary tests: executables are an absolute-path allowlist and
# production disables in-process runners.
gosec -severity medium -confidence medium \
  -exclude=G124,G204 \
  -exclude-dir=internal/postgres/sqlcgen \
  -exclude-dir=web/node_modules ./...
staticcheck -tags=integration ./cmd/... ./internal/... ./tests/security ./tests/api
govulncheck ./...
tests/security/backend-quality.sh
(
  cd web
  npm audit --audit-level=high
)

if ! command -v osv-scanner >/dev/null 2>&1; then
  printf 'Required security tool unavailable: osv-scanner\n' >&2
  exit 1
fi
go_dependencies="$(go list -deps ./...)"
if grep -q '^golang.org/x/crypto/openpgp' <<<"${go_dependencies}"; then
  printf 'GO-2026-5932 exception is invalid: openpgp became reachable\n' >&2
  exit 1
fi
osv_config="$(mktemp)"
trap 'rm -f -- "${osv_config}"' EXIT
printf '%s\n' \
  '[[IgnoredVulns]]' \
  'id = "GO-2026-5932"' \
  'ignoreUntil = 2026-10-27' \
  'reason = "x/crypto/openpgp is unmaintained, but Agent Room imports argon2 only; govulncheck and the dependency guard prove openpgp is unreachable. Review by expiry."' \
  >"${osv_config}"
osv-scanner scan source --recursive --config "${osv_config}" .
