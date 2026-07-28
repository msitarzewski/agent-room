#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'

readonly AGENTROOM_PREFIX="${AGENTROOM_PREFIX:-/opt/agentroom}"
readonly AGENTROOM_RELEASES_DIR="${AGENTROOM_RELEASES_DIR:-${AGENTROOM_PREFIX}/releases}"
readonly AGENTROOM_CURRENT_LINK="${AGENTROOM_CURRENT_LINK:-${AGENTROOM_PREFIX}/current}"
readonly AGENTROOM_CONFIG_DIR="${AGENTROOM_CONFIG_DIR:-/etc/agentroom}"
readonly AGENTROOM_CONFIG_FILE="${AGENTROOM_CONFIG_FILE:-${AGENTROOM_CONFIG_DIR}/agentroom.conf}"
readonly AGENTROOM_STATE_DIR="${AGENTROOM_STATE_DIR:-/var/lib/agentroom}"
readonly AGENTROOM_DEPLOYMENT_DIR="${AGENTROOM_DEPLOYMENT_DIR:-${AGENTROOM_STATE_DIR}/deployments}"
readonly AGENTROOM_ARTIFACT_BACKUP_DIR="${AGENTROOM_ARTIFACT_BACKUP_DIR:-/var/backups/agentroom-artifacts}"
readonly AGENTROOM_UNIT="${AGENTROOM_UNIT:-agentroom.service}"
readonly AGENTROOM_MIGRATE_UNIT="${AGENTROOM_MIGRATE_UNIT:-agentroom-migrate.service}"
readonly AGENTROOM_USER="${AGENTROOM_USER:-agentroom}"
readonly AGENTROOM_GROUP="${AGENTROOM_GROUP:-agentroom}"

log() {
  printf '%s %s\n' "$(date -u +'%Y-%m-%dT%H:%M:%SZ')" "$*" >&2
}

die() {
  log "ERROR: $*"
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "required command is unavailable: $1"
}

require_root() {
  [[ "${EUID}" -eq 0 ]] || die "this operation must run as root"
}

validate_version() {
  [[ "$1" =~ ^[0-9A-Za-z][0-9A-Za-z._+-]{0,127}$ ]] ||
    die "invalid release version: $1"
}

validate_safe_absolute_path() {
  local candidate="$1"
  [[ "${candidate}" == /* ]] || die "path must be absolute: ${candidate}"
  case "${candidate}" in
    /|/bin|/boot|/dev|/etc|/home|/lib|/lib64|/opt|/proc|/root|/run|/sbin|/srv|/sys|/tmp|/usr|/var)
      die "refusing unsafe broad path: ${candidate}"
      ;;
  esac
}

current_release_version() {
  local resolved
  [[ -L "${AGENTROOM_CURRENT_LINK}" ]] || return 1
  resolved="$(readlink -f "${AGENTROOM_CURRENT_LINK}")"
  [[ "${resolved}" == "${AGENTROOM_RELEASES_DIR}/"* ]] ||
    die "current link resolves outside release directory: ${resolved}"
  basename "${resolved}"
}

atomic_current_link() {
  local version="$1"
  local target="${AGENTROOM_RELEASES_DIR}/${version}"
  local temporary="${AGENTROOM_PREFIX}/.current.${version}.$$"

  validate_version "${version}"
  [[ -d "${target}" ]] || die "release directory does not exist: ${target}"
  ln -s "${target}" "${temporary}"
  mv -Tf "${temporary}" "${AGENTROOM_CURRENT_LINK}"
}

write_json_atomically() {
  local destination="$1"
  local temporary
  validate_safe_absolute_path "${destination}"
  temporary="$(mktemp "${destination}.tmp.XXXXXX")"
  cat >"${temporary}"
  chmod 0600 "${temporary}"
  mv -f "${temporary}" "${destination}"
}
