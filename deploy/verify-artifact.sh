#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'

usage() {
  printf 'Usage: verify-artifact.sh --artifact PATH --public-key PATH [--extract-to PATH]\n' >&2
}

artifact=""
public_key=""
extract_to=""

while (($#)); do
  case "$1" in
    --artifact) artifact="${2:?}"; shift 2 ;;
    --public-key) public_key="${2:?}"; shift 2 ;;
    --extract-to) extract_to="${2:?}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) usage; exit 2 ;;
  esac
done

[[ -f "${artifact}" ]] || { printf 'Artifact not found: %s\n' "${artifact}" >&2; exit 1; }
[[ -f "${artifact}.sha256" ]] || { printf 'Checksum file not found\n' >&2; exit 1; }
[[ -f "${artifact}.sig" ]] || { printf 'Signature file not found\n' >&2; exit 1; }
[[ -f "${public_key}" ]] || { printf 'Cosign public key not found\n' >&2; exit 1; }

for command_name in cosign file jq sha256sum tar; do
  command -v "${command_name}" >/dev/null 2>&1 || {
    printf 'Required verification tool unavailable: %s\n' "${command_name}" >&2
    exit 1
  }
done

artifact_dir="$(cd "$(dirname "${artifact}")" && pwd -P)"
artifact_base="$(basename "${artifact}")"
(
  cd "${artifact_dir}"
  checksum_entry="$(cat "${artifact_base}.sha256")"
  IFS=' ' read -r expected_checksum listed_artifact unexpected_field <<<"${checksum_entry}"
  listed_artifact="${listed_artifact#\*}"
  [[ -z "${unexpected_field:-}" &&
     "${expected_checksum:-}" =~ ^[0-9a-f]{64}$ &&
     "${listed_artifact:-}" == "${artifact_base}" ]] || {
    printf 'Checksum file has an unexpected or unsafe format\n' >&2
    exit 1
  }
  sha256sum --check --strict "${artifact_base}.sha256" >/dev/null
)
cosign verify-blob --new-bundle-format=false --key "${public_key}" \
  --signature "${artifact}.sig" "${artifact}" >/dev/null

listing="$(mktemp)"
verbose_listing="$(mktemp)"
temporary_extract=""
cleanup() {
  rm -f -- "${listing}" "${verbose_listing}"
  [[ -z "${temporary_extract}" ]] || rm -rf -- "${temporary_extract}"
}
trap cleanup EXIT

tar -tzf "${artifact}" >"${listing}"
tar -tvzf "${artifact}" >"${verbose_listing}"
while IFS= read -r entry; do
  normalized="${entry#./}"
  [[ -n "${normalized}" ]] || continue
  case "${normalized}" in
    /*|../*|*/../*|*/..)
      printf 'Unsafe archive path: %s\n' "${entry}" >&2
      exit 1
      ;;
  esac
done <"${listing}"
if grep -Eq '^[lh]' "${verbose_listing}"; then
  printf 'Release archives may not contain symlinks or hard links\n' >&2
  exit 1
fi

if [[ -n "${extract_to}" ]]; then
  [[ "${extract_to}" == /* && "${extract_to}" != "/" ]] || {
    printf 'Extraction path must be a safe absolute path\n' >&2
    exit 1
  }
  [[ -d "${extract_to}" ]] || { printf 'Extraction directory must already exist\n' >&2; exit 1; }
  [[ -z "$(find "${extract_to}" -mindepth 1 -print -quit)" ]] || {
    printf 'Extraction directory must be empty\n' >&2
    exit 1
  }
  target="${extract_to}"
else
  temporary_extract="$(mktemp -d)"
  target="${temporary_extract}"
fi

tar --no-same-owner --no-same-permissions -xzf "${artifact}" -C "${target}"
[[ -f "${target}/manifest.json" && -f "${target}/sbom.spdx.json" ]] ||
  { printf 'Required manifest or SBOM is absent\n' >&2; exit 1; }
jq -e '.schema == 1 and .target == "linux/amd64" and .signed == true and
  (.version | type == "string") and (.files | type == "array" and length > 0) and
  all(.files[]; (.path | test("^[A-Za-z0-9][A-Za-z0-9._/+@-]*$"))) and
  ([.files[].path] | length == (unique | length))' \
  "${target}/manifest.json" >/dev/null || {
  printf 'Manifest is invalid or marks the artifact unsigned\n' >&2
  exit 1
}

manifest_file_count="$(jq '.files | length' "${target}/manifest.json")"
actual_file_count="$(find "${target}" -type f ! -path "${target}/manifest.json" | wc -l | tr -d ' ')"
[[ "${actual_file_count}" == "${manifest_file_count}" ]] || {
  printf 'Archive file set does not match the signed manifest\n' >&2
  exit 1
}
while IFS= read -r -d '' actual_file; do
  actual_relative="${actual_file#"${target}/"}"
  jq -e --arg path "${actual_relative}" 'any(.files[]; .path == $path)' \
    "${target}/manifest.json" >/dev/null || {
    printf 'Archive contains an unmanifested file: %s\n' "${actual_relative}" >&2
    exit 1
  }
done < <(find "${target}" -type f ! -path "${target}/manifest.json" -print0)

while IFS=$'\t' read -r relative expected size; do
  case "${relative}" in
    /*|../*|*/../*|*/..) printf 'Unsafe manifest path\n' >&2; exit 1 ;;
  esac
  candidate="${target}/${relative}"
  [[ -f "${candidate}" ]] || { printf 'Manifest file absent: %s\n' "${relative}" >&2; exit 1; }
  actual="$(sha256sum "${candidate}" | awk '{print $1}')"
  [[ "${actual}" == "${expected}" ]] || { printf 'Digest mismatch: %s\n' "${relative}" >&2; exit 1; }
  [[ "$(stat -c '%s' "${candidate}")" == "${size}" ]] ||
    { printf 'Size mismatch: %s\n' "${relative}" >&2; exit 1; }
done < <(jq -r '.files[] | [.path,.sha256,(.size|tostring)] | @tsv' "${target}/manifest.json")

for binary in agentroomd agentroomctl; do
  [[ -x "${target}/bin/${binary}" ]] || { printf '%s is absent or non-executable\n' "${binary}" >&2; exit 1; }
  file "${target}/bin/${binary}" | grep -Eq 'ELF 64-bit.*x86-64' ||
    { printf '%s is not linux/amd64\n' "${binary}" >&2; exit 1; }
done
[[ -f "${target}/web/index.html" ]] || { printf 'Built web UI is absent\n' >&2; exit 1; }

jq -r '.version' "${target}/manifest.json"
