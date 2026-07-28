#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'

daemon_path="${1:-}"
ctl_path="${2:-}"
web_dir="${3:-}"
[[ -x "${daemon_path}" && -x "${ctl_path}" && -f "${web_dir}/index.html" ]] || {
  printf 'Usage: release-provenance-fail-closed.sh AGENTROOMD AGENTROOMCTL WEB_DIST\n' >&2
  exit 2
}
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
temporary="$(mktemp -d)"
trap 'rm -rf -- "${temporary}"' EXIT
touch "${temporary}/cosign.key"

package_attempt() {
  "${repo_root}/deploy/package-release.sh" \
    --version 0.0.0-provenance-test \
    --agentroomd "${daemon_path}" \
    --agentroomctl "${ctl_path}" \
    --web-dir "${web_dir}" \
    --output-dir "${temporary}/output" \
    --cosign-key "${temporary}/cosign.key"
}

if SOURCE_DATE_EPOCH=1785153600 package_attempt >"${temporary}/stdout" 2>"${temporary}/stderr"; then
  printf 'Production packager accepted a missing source revision\n' >&2
  exit 1
fi
grep -q 'SOURCE_REVISION must be the exact 40-character Git commit' "${temporary}/stderr"

forged_revision=0123456789abcdef0123456789abcdef01234567
if SOURCE_REVISION="${forged_revision}" SOURCE_DATE_EPOCH=1785153600 \
  package_attempt >"${temporary}/stdout" 2>"${temporary}/stderr"; then
  printf 'Production packager accepted binaries with forged provenance\n' >&2
  exit 1
fi
grep -q 'Binary provenance does not match clean SOURCE_REVISION' "${temporary}/stderr"

actual_revision="$(go version -m -json "${daemon_path}" |
  jq -r '.Settings[]? | select(.Key == "vcs.revision") | .Value' | head -n 1)"
actual_modified="$(go version -m -json "${daemon_path}" |
  jq -r '.Settings[]? | select(.Key == "vcs.modified") | .Value' | head -n 1)"
if [[ ! "${actual_revision}" =~ ^[0-9a-f]{40}$ ]]; then
  [[ "${actual_modified}" == "true" ]] || {
    printf 'Test binary lacks both exact and dirty VCS provenance\n' >&2
    exit 1
  }
  printf 'Production release provenance fails closed for uncommitted binaries\n'
  exit 0
fi
if SOURCE_REVISION="${actual_revision}" SOURCE_DATE_EPOCH=1785153600 \
  package_attempt >"${temporary}/stdout" 2>"${temporary}/stderr"; then
  printf 'Production packager accepted a web build without matching provenance\n' >&2
  exit 1
fi
if [[ "${actual_modified}" == "true" ]]; then
  grep -q 'Binary provenance does not match clean SOURCE_REVISION' "${temporary}/stderr"
else
  grep -q 'Web build provenance does not match SOURCE_REVISION and SOURCE_DATE_EPOCH' "${temporary}/stderr"
fi

printf 'Production release provenance fails closed\n'
