#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=deploy/lib/common.sh
source "${script_dir}/lib/common.sh"

readonly POSTGRESQL_MAJOR=18
readonly POSTGRESQL_APT_KEY_URL='https://www.postgresql.org/media/keys/ACCC4CF8.asc'
readonly POSTGRESQL_APT_KEY_FINGERPRINT='B97B0AFCAA1A47F044F244A07FCC7D46ACCC4CF8'
readonly COSIGN_VERSION='3.0.6'
readonly COSIGN_AMD64_SHA256='e16e8eb815f8b1b3cee3e678874393c286f19dd59e9ac5da95e428f970ef00f3'
bootstrap_temporary=""

cleanup() {
  [[ -z "${bootstrap_temporary}" ]] || rm -rf -- "${bootstrap_temporary}"
}
trap cleanup EXIT

usage() {
  cat >&2 <<'EOF'
Usage:
  install-host.sh --apply
  install-host.sh --bootstrap --apply

--apply creates the Agent Room service identity, directories, systemd units,
and AppArmor profiles.

--bootstrap additionally installs verified Ubuntu/amd64 production
prerequisites, PostgreSQL 18, pgBackRest, and Cosign before preparing the host.
It does not create application database credentials, OIDC credentials, TLS
credentials, or start Agent Room.
EOF
}

bootstrap=0
apply=0
while (($#)); do
  case "$1" in
    --bootstrap) bootstrap=1; shift ;;
    --apply) apply=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) usage; exit 2 ;;
  esac
done
((apply == 1)) || { usage; exit 2; }
require_root

bootstrap_prerequisites() {
  [[ -r /etc/os-release ]] || die "Ubuntu release metadata is unavailable"
  # shellcheck disable=SC1091
  source /etc/os-release
  [[ "${ID:-}" == "ubuntu" ]] || die "bootstrap supports Ubuntu only"
  [[ "${VERSION_CODENAME:-}" =~ ^[a-z][a-z0-9-]*$ ]] ||
    die "Ubuntu VERSION_CODENAME is missing or unsafe"

  for command_name in apt-get dpkg install mktemp sha256sum systemctl; do
    require_command "${command_name}"
  done
  [[ "$(dpkg --print-architecture)" == "amd64" ]] ||
    die "bootstrap supports Ubuntu amd64 only"

  export DEBIAN_FRONTEND=noninteractive
  apt-get update
  apt-get install --yes --no-install-recommends \
    apparmor apparmor-utils ca-certificates curl file gnupg jq nftables postgresql-common ripgrep
  for command_name in curl gpg; do
    require_command "${command_name}"
  done

  local key_path key_fingerprint source_path cosign_path cosign_url postgres_config
  bootstrap_temporary="$(mktemp -d)"

  key_path="${bootstrap_temporary}/apt.postgresql.org.asc"
  curl --proto '=https' --tlsv1.2 --fail --location --silent --show-error \
    "${POSTGRESQL_APT_KEY_URL}" --output "${key_path}"
  key_fingerprint="$(gpg --batch --show-keys --with-colons "${key_path}" |
    awk -F: '$1 == "fpr" { print $10; exit }')"
  [[ "${key_fingerprint}" == "${POSTGRESQL_APT_KEY_FINGERPRINT}" ]] ||
    die "PostgreSQL APT signing-key fingerprint does not match the pinned value"
  install -d -o root -g root -m 0755 /usr/share/postgresql-common/pgdg
  install -o root -g root -m 0644 "${key_path}" \
    /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc

  source_path="${bootstrap_temporary}/pgdg.list"
  printf 'deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc] https://apt.postgresql.org/pub/repos/apt %s-pgdg main\n' \
    "${VERSION_CODENAME}" >"${source_path}"
  install -o root -g root -m 0644 "${source_path}" /etc/apt/sources.list.d/pgdg.list

  apt-get update
  apt-get install --yes --no-install-recommends \
    "postgresql-${POSTGRESQL_MAJOR}" "postgresql-client-${POSTGRESQL_MAJOR}" pgbackrest

  cosign_path="${bootstrap_temporary}/cosign_${COSIGN_VERSION}_amd64.deb"
  cosign_url="https://github.com/sigstore/cosign/releases/download/v${COSIGN_VERSION}/cosign_${COSIGN_VERSION}_amd64.deb"
  curl --proto '=https' --tlsv1.2 --fail --location --silent --show-error \
    "${cosign_url}" --output "${cosign_path}"
  printf '%s  %s\n' "${COSIGN_AMD64_SHA256}" "${cosign_path}" |
    sha256sum --check --strict
  apt-get install --yes --no-install-recommends "${cosign_path}"

  postgres_config="/etc/postgresql/${POSTGRESQL_MAJOR}/main/conf.d/agentroom.conf"
  [[ -d "$(dirname "${postgres_config}")" ]] ||
    die "PostgreSQL ${POSTGRESQL_MAJOR} main cluster configuration is absent"
  cat >"${bootstrap_temporary}/agentroom.conf" <<'EOF'
# Agent Room host baseline. Keep PostgreSQL private and resource-bounded.
listen_addresses = '127.0.0.1'
shared_buffers = '256MB'
max_connections = 50
work_mem = '4MB'
maintenance_work_mem = '64MB'
password_encryption = 'scram-sha-256'
EOF
  install -o root -g postgres -m 0640 "${bootstrap_temporary}/agentroom.conf" "${postgres_config}"
  systemctl enable postgresql
  systemctl restart postgresql
  systemctl is-active --quiet postgresql ||
    die "PostgreSQL did not become active after applying the Agent Room baseline"
  log "Ubuntu prerequisites, PostgreSQL ${POSTGRESQL_MAJOR}, pgBackRest, and Cosign ${COSIGN_VERSION} installed"
}

if ((bootstrap == 1)); then
  bootstrap_prerequisites
fi

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
