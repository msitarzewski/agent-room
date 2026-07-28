#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
source "${script_dir}/lib/common.sh"

target=""
public_url=""
while (($#)); do
  case "$1" in
    --version) target="${2:?}"; shift 2 ;;
    --public-url) public_url="${2:?}"; shift 2 ;;
    -h|--help)
      printf 'Usage: rollback.sh [--version VERSION] --public-url HTTPS_ORIGIN\n'
      exit 0
      ;;
    *) die "unknown argument: $1" ;;
  esac
done

require_root
require_command jq
require_command systemctl
[[ "${public_url}" =~ ^https://[^/]+$ ]] || die "public URL must be an HTTPS origin"

if [[ -z "${target}" ]]; then
  rollback_record="${AGENTROOM_DEPLOYMENT_DIR}/rollback-target.json"
  [[ -f "${rollback_record}" ]] || die "no recorded rollback target exists"
  target="$(jq -er '.version' "${rollback_record}")"
fi
validate_version "${target}"
[[ -d "${AGENTROOM_RELEASES_DIR}/${target}" ]] || die "rollback release is not installed: ${target}"
from="$(current_release_version)" || die "there is no current release"
[[ "${from}" != "${target}" ]] || die "target is already current"

switched=0
restore_on_error() {
  local status=$?
  trap - ERR
  if ((switched == 1)); then
    log "rollback target failed verification; restoring ${from}"
    systemctl stop "${AGENTROOM_UNIT}" || true
    atomic_current_link "${from}"
    systemctl start "${AGENTROOM_UNIT}" || true
  fi
  exit "${status}"
}
trap restore_on_error ERR

systemctl stop "${AGENTROOM_UNIT}"
atomic_current_link "${target}"
switched=1
systemctl start "${AGENTROOM_UNIT}"
systemctl is-active --quiet "${AGENTROOM_UNIT}" || die "rolled-back service did not become active"
"${script_dir}/smoke.sh" --public-url "${public_url}"
"${script_dir}/migrate.sh" verify

record="${AGENTROOM_DEPLOYMENT_DIR}/rollback-$(date -u +'%Y%m%dT%H%M%SZ').json"
jq -cn --arg from "${from}" --arg to "${target}" \
  --arg at "$(date -u +'%Y-%m-%dT%H:%M:%SZ')" \
  '{schema:1,status:"succeeded",from:$from,to:$to,rolled_back_at:$at}' |
  write_json_atomically "${record}"
trap - ERR
switched=0
log "rolled back from ${from} to ${target}; database was not down-migrated"
