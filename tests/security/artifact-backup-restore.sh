#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
temporary="$(mktemp -d)"
trap 'rm -rf -- "${temporary}"' EXIT
source_dir="${temporary}/source"
backup_dir="${temporary}/backups"
restore_dir="${temporary}/restored"
mkdir -p "${source_dir}/sha256/ab"
printf 'immutable artifact payload\n' >"${source_dir}/sha256/ab/abcdef"

record="$("${repo_root}/deploy/artifact-backup.sh" \
  --source "${source_dir}" --backup-dir "${backup_dir}" --retention 2)"
archive="$(jq -er '.archive' <<<"${record}")"
archive_sha256="$(jq -er '.sha256' <<<"${record}")"
[[ "$(jq -er '.file_count' <<<"${record}")" == "1" ]]

printf 'mutated live copy\n' >"${source_dir}/sha256/ab/abcdef"
"${repo_root}/deploy/artifact-restore.sh" \
  --archive "${archive}" --sha256 "${archive_sha256}" \
  --target "${restore_dir}" --confirm RESTORE_TO_ISOLATED_TARGET >/dev/null
grep -qx 'immutable artifact payload' "${restore_dir}/sha256/ab/abcdef"
[[ -f "${restore_dir}/.backup-manifest.json" ]]

corrupt="${temporary}/corrupt.tar.gz"
cp "${archive}" "${corrupt}"
chmod 0600 "${corrupt}"
printf 'corruption' >>"${corrupt}"
if "${repo_root}/deploy/artifact-restore.sh" \
  --archive "${corrupt}" --sha256 "${archive_sha256}" \
  --target "${temporary}/corrupt-restore" --confirm RESTORE_TO_ISOLATED_TARGET \
  >/dev/null 2>&1; then
  printf 'Corrupt artifact backup was accepted\n' >&2
  exit 1
fi

ln -s /etc/passwd "${source_dir}/unsafe-link"
if "${repo_root}/deploy/artifact-backup.sh" \
  --source "${source_dir}" --backup-dir "${backup_dir}" --retention 2 \
  >/dev/null 2>&1; then
  printf 'Artifact backup accepted a symlink\n' >&2
  exit 1
fi
rm "${source_dir}/unsafe-link"

for iteration in 1 2 3; do
  printf '%s\n' "${iteration}" >"${source_dir}/sha256/ab/abcdef"
  "${repo_root}/deploy/artifact-backup.sh" \
    --source "${source_dir}" --backup-dir "${backup_dir}" --retention 2 >/dev/null
done
backup_count="$(find "${backup_dir}" -maxdepth 1 -type f -name 'artifact-*.tar.gz' | wc -l | tr -d ' ')"
[[ "${backup_count}" == "2" ]] || {
  printf 'Artifact backup retention kept %s archives, want 2\n' "${backup_count}" >&2
  exit 1
}
printf 'Artifact backup, retention, and isolated restore passed\n'
