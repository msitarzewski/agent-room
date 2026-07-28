#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
source "${script_dir}/lib/common.sh"

[[ "${1:-}" == "--apply" ]] || {
  printf 'Usage: install-host.sh --apply\n' >&2
  printf 'This creates the agentroom service identity, directories, systemd units, and AppArmor profiles.\n' >&2
  exit 2
}
require_root
for command_name in apparmor_parser getent install systemctl useradd; do
  require_command "${command_name}"
done

if ! getent group "${AGENTROOM_GROUP}" >/dev/null; then
  groupadd --system "${AGENTROOM_GROUP}"
fi
if ! getent passwd "${AGENTROOM_USER}" >/dev/null; then
  useradd --system --gid "${AGENTROOM_GROUP}" --home-dir "${AGENTROOM_STATE_DIR}" \
    --shell /usr/sbin/nologin "${AGENTROOM_USER}"
fi

install -d -o root -g root -m 0755 "${AGENTROOM_PREFIX}" "${AGENTROOM_RELEASES_DIR}"
install -d -o root -g "${AGENTROOM_GROUP}" -m 0750 "${AGENTROOM_CONFIG_DIR}"
install -d -o root -g root -m 0700 "${AGENTROOM_CONFIG_DIR}/credentials"
install -d -o "${AGENTROOM_USER}" -g "${AGENTROOM_GROUP}" -m 0700 \
  "${AGENTROOM_STATE_DIR}" "${AGENTROOM_DEPLOYMENT_DIR}" "${AGENTROOM_STATE_DIR}/artifacts"
install -d -o root -g root -m 0700 "${AGENTROOM_ARTIFACT_BACKUP_DIR}"
install -d -o root -g root -m 0755 /usr/local/libexec/agentroom

for script in artifact-backup.sh artifact-restore.sh backup.sh deploy.sh doctor.sh migrate.sh restore.sh rollback.sh smoke.sh verify-artifact.sh; do
  install -o root -g root -m 0755 "${script_dir}/${script}" "/usr/local/libexec/agentroom/${script}"
done
install -d -o root -g root -m 0755 /usr/local/libexec/agentroom/lib
install -o root -g root -m 0644 "${script_dir}/lib/common.sh" /usr/local/libexec/agentroom/lib/common.sh
install -o root -g root -m 0644 "${script_dir}/systemd/agentroom.service" /etc/systemd/system/agentroom.service
install -o root -g root -m 0644 "${script_dir}/systemd/agentroom-migrate.service" /etc/systemd/system/agentroom-migrate.service

if [[ ! -e "${AGENTROOM_CONFIG_FILE}" ]]; then
  install -o root -g "${AGENTROOM_GROUP}" -m 0640 \
    "${script_dir}/config/agentroom.conf.example" "${AGENTROOM_CONFIG_FILE}"
  log "installed example config; replace .invalid values before starting"
fi

install -o root -g root -m 0644 "${script_dir}/apparmor/opt.agentroom.agentroomd" \
  /etc/apparmor.d/opt.agentroom.agentroomd
install -o root -g root -m 0644 "${script_dir}/apparmor/opt.agentroom.agentroomctl" \
  /etc/apparmor.d/opt.agentroom.agentroomctl
apparmor_parser -r /etc/apparmor.d/opt.agentroom.agentroomd
apparmor_parser -r /etc/apparmor.d/opt.agentroom.agentroomctl
systemctl daemon-reload
systemctl enable "${AGENTROOM_UNIT}"
log "host installation complete; service was enabled but not started"
log "provision encrypted credentials and production config, then run doctor.sh"
