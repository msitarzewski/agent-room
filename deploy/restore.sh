#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)/lib/common.sh"

stanza="${PGBACKREST_STANZA:-agentroom}"
record=""
target=""
artifact_target=""
confirm=""

while (($#)); do
  case "$1" in
    --stanza) stanza="${2:?}"; shift 2 ;;
    --record) record="${2:?}"; shift 2 ;;
    --target) target="${2:?}"; shift 2 ;;
    --artifact-target) artifact_target="${2:?}"; shift 2 ;;
    --confirm) confirm="${2:?}"; shift 2 ;;
    -h|--help)
      printf 'Usage: restore.sh --record BACKUP_JSON --target NEW_DB_DIR --artifact-target NEW_ARTIFACT_DIR --confirm RESTORE_TO_ISOLATED_TARGET\n'
      exit 0
      ;;
    *) die "unknown argument: $1" ;;
  esac
done

require_root
require_command pgbackrest
require_command runuser
require_command jq
backup_user="${PGBACKREST_USER:-postgres}"
[[ "${confirm}" == "RESTORE_TO_ISOLATED_TARGET" ]] ||
  die "explicit isolated-restore confirmation is required"
validate_safe_absolute_path "${record}"
[[ -f "${record}" ]] || die "coordinated backup record does not exist"
[[ "$(jq -er '.schema' "${record}")" == "2" ]] || die "unsupported coordinated backup record"
record_stanza="$(jq -er '.database.stanza' "${record}")"
[[ "${record_stanza}" == "${stanza}" ]] || die "backup record stanza does not match requested stanza"
backup_set="$(jq -er '.database.label' "${record}")"
artifact_archive="$(jq -er '.artifacts.archive' "${record}")"
artifact_sha256="$(jq -er '.artifacts.sha256' "${record}")"
[[ "${backup_set}" =~ ^[0-9]{8}-[0-9]{6}[FDI](_[0-9]{8}-[0-9]{6}[DI])?$ ]] ||
  die "backup label format is invalid"
validate_safe_absolute_path "${target}"
validate_safe_absolute_path "${artifact_target}"
case "${target}" in
  /var/lib/postgresql/*|/var/lib/agentroom/*)
    die "this script refuses production data paths; restore to an isolated target"
    ;;
esac
case "${artifact_target}" in
  /var/lib/postgresql/*|/var/lib/agentroom/*)
    die "this script refuses production data paths; restore to an isolated target"
    ;;
esac
[[ "${target}" != "${artifact_target}" ]] || die "database and artifact restore targets must differ"
mkdir -p "${target}"
[[ -z "$(find "${target}" -mindepth 1 -print -quit)" ]] ||
  die "restore target must be empty"
chown "${backup_user}:${backup_user}" "${target}"
chmod 0700 "${target}"

runuser -u "${backup_user}" -- pgbackrest \
  --stanza="${stanza}" --set="${backup_set}" --pg1-path="${target}" restore
[[ -f "${target}/PG_VERSION" ]] || die "restore completed without a PostgreSQL data directory"
"$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)/artifact-restore.sh" \
  --archive "${artifact_archive}" \
  --sha256 "${artifact_sha256}" \
  --target "${artifact_target}" \
  --confirm RESTORE_TO_ISOLATED_TARGET >/dev/null
chown -R "${AGENTROOM_USER}:${AGENTROOM_GROUP}" "${artifact_target}"
find "${artifact_target}" -type d -exec chmod 0750 {} +
find "${artifact_target}" -type f -exec chmod 0640 {} +
log "isolated coordinated restore completed at ${target} and ${artifact_target}; no production service was modified"
