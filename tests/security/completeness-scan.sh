#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
cd "${repo_root}"
command -v rg >/dev/null 2>&1 || { printf 'ripgrep is required\n' >&2; exit 1; }

production_paths=()
for candidate in cmd internal web/src deploy; do
  [[ -e "${candidate}" ]] && production_paths+=("${candidate}")
done
(( ${#production_paths[@]} > 0 )) || {
  printf 'No production source paths were found\n' >&2
  exit 1
}

marker_pattern='\b(TODO|FIXME|XXX|HACK|STUB|NotImplemented)\b|not[ -]?implemented|placeholder[[:space:]]+(implementation|code|value)'
skip_pattern='t\.Skip\(|test\.skip\(|describe\.skip\(|\.only\(|xdescribe\(|(^|[^[:alnum:]_])xit\('
allowed_skip_pattern='AGENTROOM_TEST_DATABASE_URL is not set|abstract Unix sockets are Linux-specific'
suppression_pattern='#nosec|//nolint|istanbul ignore|coverage ignore'

failed=0
for pattern in "${marker_pattern}" "${suppression_pattern}"; do
  if rg -n -i --glob '!**/*.md' --glob '!**/*.map' --glob '!deploy/config/agentroom.conf.example' \
    "${pattern}" "${production_paths[@]}"; then
    failed=1
  fi
done
skip_findings="$(rg -n -i --glob '!**/*.md' --glob '!**/*.map' \
  "${skip_pattern}" "${production_paths[@]}" 2>/dev/null |
  rg -v "${allowed_skip_pattern}" || true)"
if [[ -n "${skip_findings}" ]]; then
  printf '%s\n' "${skip_findings}"
  failed=1
fi

if rg -n --glob '*.go' 'panic\(\s*"[^"]*(not implemented|unreachable)' cmd internal 2>/dev/null; then
  failed=1
fi

((failed == 0)) || {
  printf 'Incomplete, skipped, or suppressed production references require review\n' >&2
  exit 1
}
printf 'No incomplete, skipped, or suppressed production references found\n'
