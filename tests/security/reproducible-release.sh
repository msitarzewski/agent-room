#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'

daemon_path="${1:-}"
ctl_path="${2:-}"
web_dir="${3:-}"
[[ -x "${daemon_path}" && -x "${ctl_path}" && -f "${web_dir}/index.html" ]] || {
  printf 'Usage: reproducible-release.sh AGENTROOMD AGENTROOMCTL WEB_DIST\n' >&2
  exit 2
}
for command_name in cmp jq sha256sum syft tar; do
  command -v "${command_name}" >/dev/null 2>&1 || {
    printf 'Required reproducibility tool unavailable: %s\n' "${command_name}" >&2
    exit 1
  }
done

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
temporary="$(mktemp -d)"
trap 'rm -rf -- "${temporary}"' EXIT
mkdir -p "${temporary}/first" "${temporary}/second"
common=(
  --version 0.0.0-reproducibility
  --agentroomd "${daemon_path}"
  --agentroomctl "${ctl_path}"
  --web-dir "${web_dir}"
  --development-unsigned
)
for output in first second; do
  SOURCE_REVISION=0123456789abcdef0123456789abcdef01234567 \
  SOURCE_DATE_EPOCH=1785153600 \
    "${repo_root}/deploy/package-release.sh" "${common[@]}" \
      --output-dir "${temporary}/${output}" >/dev/null
done

first="${temporary}/first/agentroom-0.0.0-reproducibility-linux-amd64.tar.gz"
second="${temporary}/second/agentroom-0.0.0-reproducibility-linux-amd64.tar.gz"
cmp --silent "${first}" "${second}" || {
  printf 'Identical release inputs did not produce byte-identical archives\n' >&2
  sha256sum "${first}" "${second}" >&2
  exit 1
}
cmp --silent "${first}.sha256" "${second}.sha256" || {
  printf 'Identical release inputs did not produce byte-identical checksums\n' >&2
  exit 1
}
archive_contents="${temporary}/archive-contents.txt"
tar -tzf "${first}" >"${archive_contents}"
for legal_path in ./LICENSE ./NOTICE ./THIRD_PARTY_NOTICES.md; do
  grep -qx "${legal_path}" "${archive_contents}" || {
    printf 'Release archive is missing required license material: %s\n' "${legal_path}" >&2
    exit 1
  }
done
grep -q '^\./third_party/.*/LICENSE' "${archive_contents}" || {
  printf 'Release archive is missing third-party license texts\n' >&2
  exit 1
}
printf 'Release packaging is byte-for-byte reproducible\n'
