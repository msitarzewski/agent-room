#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'
umask 0077
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)/lib/common.sh"

source_dir="${AGENTROOM_STATE_DIR}/artifacts"
backup_dir="${AGENTROOM_ARTIFACT_BACKUP_DIR:-/var/backups/agentroom-artifacts}"
retention="${AGENTROOM_ARTIFACT_BACKUP_RETENTION:-14}"
while (($#)); do
  case "$1" in
    --source) source_dir="${2:?}"; shift 2 ;;
    --backup-dir) backup_dir="${2:?}"; shift 2 ;;
    --retention) retention="${2:?}"; shift 2 ;;
    -h|--help)
      printf 'Usage: artifact-backup.sh [--source DIR] [--backup-dir DIR] [--retention COUNT]\n'
      exit 0
      ;;
    *) die "unknown argument: $1" ;;
  esac
done
for command_name in find gzip jq sha256sum tar; do require_command "${command_name}"; done
validate_safe_absolute_path "${source_dir}"
validate_safe_absolute_path "${backup_dir}"
[[ "${retention}" =~ ^[1-9][0-9]*$ && "${retention}" -le 365 ]] ||
  die "artifact backup retention must be between 1 and 365"
[[ -d "${source_dir}" ]] || die "artifact source directory does not exist"
[[ -z "$(find "${source_dir}" -mindepth 1 ! -type d ! -type f -print -quit)" ]] ||
  die "artifact source may contain only regular files and directories"
mkdir -p "${backup_dir}"
chmod 0700 "${backup_dir}"
source_real="$(cd "${source_dir}" && pwd -P)"
backup_real="$(cd "${backup_dir}" && pwd -P)"
[[ "${backup_real}" != "${source_real}" && "${backup_real}" != "${source_real}/"* ]] ||
  die "artifact backup directory may not be inside the artifact source"

backup_id="$(date -u +'%Y%m%d-%H%M%S')-$$"
stage="$(mktemp -d "${backup_dir}/.snapshot.XXXXXX")"
archive_temporary="${backup_dir}/.${backup_id}.tar.gz.tmp"
trap 'rm -rf -- "${stage}"; rm -f -- "${archive_temporary}"' EXIT
mkdir -p "${stage}/artifacts"
cp -a "${source_dir}/." "${stage}/artifacts/"

files_json="${stage}/.files.jsonl"
: >"${files_json}"
while IFS= read -r -d '' file_path; do
  relative="${file_path#"${stage}/artifacts/"}"
  [[ "${relative}" =~ ^[A-Za-z0-9][A-Za-z0-9._/+@-]*$ ]] ||
    die "artifact path contains unsupported characters: ${relative}"
  digest="$(sha256sum "${file_path}" | awk '{print $1}')"
  size="$(wc -c <"${file_path}" | tr -d ' ')"
  jq -cn --arg path "${relative}" --arg sha256 "${digest}" --argjson size "${size}" \
    '{path:$path,sha256:$sha256,size:$size}' >>"${files_json}"
done < <(find "${stage}/artifacts" -type f -print0 | sort -z)
jq -s \
  --arg backup_id "${backup_id}" \
  --arg created_at "$(date -u +'%Y-%m-%dT%H:%M:%SZ')" \
  '{schema:1,backup_id:$backup_id,created_at:$created_at,files:.}' \
  "${files_json}" >"${stage}/manifest.json"
rm -f "${files_json}"

tar -C "${stage}" -cf - manifest.json artifacts | gzip -n >"${archive_temporary}"
archive="${backup_dir}/artifact-${backup_id}.tar.gz"
mv -f "${archive_temporary}" "${archive}"
chmod 0400 "${archive}"
archive_sha256="$(sha256sum "${archive}" | awk '{print $1}')"

find "${backup_dir}" -maxdepth 1 -type f -name 'artifact-*.tar.gz' -print |
  sort -r | tail -n "+$((retention + 1))" |
  while IFS= read -r expired; do
    [[ -n "${expired}" ]] || continue
    rm -f -- "${expired}"
  done

jq -cn \
  --arg backup_id "${backup_id}" \
  --arg archive "${archive}" \
  --arg sha256 "${archive_sha256}" \
  --argjson file_count "$(jq '.files | length' "${stage}/manifest.json")" \
  '{schema:1,backup_id:$backup_id,archive:$archive,sha256:$sha256,file_count:$file_count}'
