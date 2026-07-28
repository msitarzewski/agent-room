#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)/lib/common.sh"

stanza="${PGBACKREST_STANZA:-agentroom}"
backup_type="incr"
output_file=""
artifact_dir="${AGENTROOM_STATE_DIR}/artifacts"
artifact_backup_dir="${AGENTROOM_ARTIFACT_BACKUP_DIR:-/var/backups/agentroom-artifacts}"
artifact_retention="${AGENTROOM_ARTIFACT_BACKUP_RETENTION:-14}"
confirm_quiesced=""

while (($#)); do
  case "$1" in
    --stanza) stanza="${2:?}"; shift 2 ;;
    --type) backup_type="${2:?}"; shift 2 ;;
    --output) output_file="${2:?}"; shift 2 ;;
    --artifact-dir) artifact_dir="${2:?}"; shift 2 ;;
    --artifact-backup-dir) artifact_backup_dir="${2:?}"; shift 2 ;;
    --artifact-retention) artifact_retention="${2:?}"; shift 2 ;;
    --confirm-quiesced) confirm_quiesced="${2:?}"; shift 2 ;;
    -h|--help)
      printf 'Usage: backup.sh [--stanza NAME] [--type full|diff|incr] [--output FILE] --confirm-quiesced AGENTROOM_STOPPED\n'
      exit 0
      ;;
    *) die "unknown argument: $1" ;;
  esac
done

[[ "${stanza}" =~ ^[A-Za-z0-9_-]+$ ]] || die "invalid pgBackRest stanza"
case "${backup_type}" in full|diff|incr) ;; *) die "invalid backup type" ;; esac
[[ "${confirm_quiesced}" == "AGENTROOM_STOPPED" ]] ||
  die "coordinated backup requires a quiesced Agent Room service"
require_command pgbackrest
require_command jq
backup_user="${PGBACKREST_USER:-postgres}"
run_pgbackrest() {
  if [[ "${EUID}" -eq 0 ]]; then
    require_command runuser
    runuser -u "${backup_user}" -- pgbackrest "$@"
  else
    pgbackrest "$@"
  fi
}

log "checking pgBackRest stanza ${stanza}"
run_pgbackrest --stanza="${stanza}" check
log "creating ${backup_type} backup"
run_pgbackrest --stanza="${stanza}" --type="${backup_type}" backup
info="$(run_pgbackrest --stanza="${stanza}" --output=json info)"
label="$(jq -er '.[0].backup[-1].label' <<<"${info}")" || die "unable to identify completed backup"
status="$(jq -er '.[0].status.code' <<<"${info}")" || die "unable to read backup status"
[[ "${status}" == "0" ]] || die "pgBackRest reports an unhealthy stanza after backup"

artifact_record="$("$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)/artifact-backup.sh" \
  --source "${artifact_dir}" \
  --backup-dir "${artifact_backup_dir}" \
  --retention "${artifact_retention}")"
record="$(jq -cn \
  --arg stanza "${stanza}" \
  --arg label "${label}" \
  --arg type "${backup_type}" \
  --arg created_at "$(date -u +'%Y-%m-%dT%H:%M:%SZ')" \
  --argjson artifact "${artifact_record}" \
  '{schema:2,created_at:$created_at,
    database:{stanza:$stanza,label:$label,type:$type},
    artifacts:$artifact}')"

if [[ -n "${output_file}" ]]; then
  validate_safe_absolute_path "${output_file}"
  mkdir -p "$(dirname "${output_file}")"
  printf '%s\n' "${record}" | write_json_atomically "${output_file}"
fi
printf '%s\n' "${record}"
