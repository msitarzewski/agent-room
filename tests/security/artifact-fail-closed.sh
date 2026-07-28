#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
verifier="${repo_root}/deploy/verify-artifact.sh"
tmp="$(mktemp -d)"
trap 'rm -rf -- "${tmp}"' EXIT

mkdir -p "${tmp}/payload"
printf 'unsigned\n' >"${tmp}/payload/manifest.json"
tar -czf "${tmp}/unsigned.tar.gz" -C "${tmp}/payload" .

if "${verifier}" --artifact "${tmp}/unsigned.tar.gz" --public-key "${tmp}/missing.pub" \
  >"${tmp}/stdout" 2>"${tmp}/stderr"; then
  printf 'Verifier accepted an unsigned artifact\n' >&2
  exit 1
fi
grep -q 'Checksum file not found' "${tmp}/stderr" || {
  printf 'Verifier did not fail for the expected missing integrity evidence\n' >&2
  exit 1
}
printf 'Artifact verifier rejected incomplete integrity evidence\n'
