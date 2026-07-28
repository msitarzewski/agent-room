#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'
umask 0077
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)/lib/common.sh"

archive=""
expected_sha256=""
target=""
confirm=""
while (($#)); do
  case "$1" in
    --archive) archive="${2:?}"; shift 2 ;;
    --sha256) expected_sha256="${2:?}"; shift 2 ;;
    --target) target="${2:?}"; shift 2 ;;
    --confirm) confirm="${2:?}"; shift 2 ;;
    -h|--help)
      printf 'Usage: artifact-restore.sh --archive FILE --sha256 HEX --target NEW_DIR --confirm RESTORE_TO_ISOLATED_TARGET\n'
      exit 0
      ;;
    *) die "unknown argument: $1" ;;
  esac
done
for command_name in find jq sha256sum tar; do require_command "${command_name}"; done
[[ "${confirm}" == "RESTORE_TO_ISOLATED_TARGET" ]] ||
  die "explicit isolated-restore confirmation is required"
[[ "${expected_sha256}" =~ ^[0-9a-f]{64}$ ]] || die "invalid artifact backup checksum"
validate_safe_absolute_path "${archive}"
validate_safe_absolute_path "${target}"
case "${target}" in
  /var/lib/agentroom|/var/lib/agentroom/*)
    die "this script refuses the production Agent Room state path"
    ;;
esac
[[ -f "${archive}" ]] || die "artifact backup archive does not exist"
[[ ! -e "${target}" ]] || die "artifact restore target must not already exist"
[[ "$(sha256sum "${archive}" | awk '{print $1}')" == "${expected_sha256}" ]] ||
  die "artifact backup checksum mismatch"

listing="$(mktemp)"
verbose_listing="$(mktemp)"
stage="$(mktemp -d "$(dirname "${target}")/.agentroom-artifact-restore.XXXXXX")"
trap 'rm -f -- "${listing}" "${verbose_listing}"; [[ -z "${stage}" ]] || rm -rf -- "${stage}"' EXIT
tar -tzf "${archive}" >"${listing}"
tar -tvzf "${archive}" >"${verbose_listing}"
if grep -Eq '(^|/)\.\.(/|$)|^/' "${listing}"; then
  die "artifact backup contains an unsafe path"
fi
if awk '$1 !~ /^[-d]/ {exit 1}' "${verbose_listing}"; then :; else
  die "artifact backup contains a link or special file"
fi
tar --no-same-owner --no-same-permissions -xzf "${archive}" -C "${stage}"
[[ -f "${stage}/manifest.json" && -d "${stage}/artifacts" ]] ||
  die "artifact backup structure is invalid"
[[ "$(jq -er '.schema' "${stage}/manifest.json")" == "1" ]] ||
  die "artifact backup manifest schema is invalid"
while IFS= read -r item; do
  relative="$(jq -er '.path' <<<"${item}")"
  digest="$(jq -er '.sha256' <<<"${item}")"
  size="$(jq -er '.size' <<<"${item}")"
  [[ "${relative}" =~ ^[A-Za-z0-9][A-Za-z0-9._/+@-]*$ && "${digest}" =~ ^[0-9a-f]{64}$ ]] ||
    die "artifact manifest entry is invalid"
  restored="${stage}/artifacts/${relative}"
  [[ -f "${restored}" && ! -L "${restored}" ]] || die "manifested artifact is missing"
  [[ "$(sha256sum "${restored}" | awk '{print $1}')" == "${digest}" ]] ||
    die "restored artifact digest mismatch"
  [[ "$(wc -c <"${restored}" | tr -d ' ')" == "${size}" ]] || die "restored artifact size mismatch"
done < <(jq -c '.files[]' "${stage}/manifest.json")
manifest_count="$(jq '.files | length' "${stage}/manifest.json")"
actual_count="$(find "${stage}/artifacts" -type f | wc -l | tr -d ' ')"
[[ "${actual_count}" == "${manifest_count}" ]] || die "artifact backup contains unmanifested files"
mv "${stage}/artifacts" "${target}"
install -m 0400 "${stage}/manifest.json" "${target}/.backup-manifest.json"
printf '%s\n' "${target}"
