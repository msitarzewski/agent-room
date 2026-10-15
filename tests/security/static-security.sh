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
# Scan current source, including tracked edits and nonignored untracked files.
# Ignored runtime credentials and generated/dependency data are not source.
source_scan="$(mktemp -d)"
trap 'rm -rf -- "${source_scan}"' EXIT
mkdir "${source_scan}/source"
git ls-files -z --cached --others --exclude-standard >"${source_scan}/inventory"
while IFS= read -r -d '' source_file; do
  source_target="${source_scan}/source/${source_file}"
  if [[ -L "${source_file}" ]]; then
    mkdir -p -- "${source_target%/*}"
    # Git records a symlink's target text, not the target file's contents.
    readlink "./${source_file}" >"${source_target}"
  elif [[ -f "${source_file}" ]]; then
    mkdir -p -- "${source_target%/*}"
    cp -- "${source_file}" "${source_target}"
  fi
done <"${source_scan}/inventory"
gitleaks dir --redact --config "${repo_root}/.gitleaks.toml" "${source_scan}/source"
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
  printf 'golang.org/x/crypto/openpgp became reachable (GO-2026-5932)\n' >&2
  exit 1
fi
# No ignore list: osv-scanner's Go call analysis reports unreachable x/crypto
# advisories such as GO-2026-5932 as uncalled, and the guard above keeps
# openpgp out of the build.
osv-scanner scan source --recursive .
