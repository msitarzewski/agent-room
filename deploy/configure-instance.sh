#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'
umask 077
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
# shellcheck source=deploy/lib/common.sh
source "${script_dir}/lib/common.sh"

readonly DATABASE_NAME=agentroom
readonly DATABASE_USER=agentroom
readonly POSTGRESQL_MAJOR=18

usage() {
  cat >&2 <<'EOF'
Usage:
  configure-instance.sh \
    --public-url https://agentroom.example.invalid \
    --oidc-issuer https://identity.example.invalid \
    --oidc-client-id CLIENT_ID \
    [--oidc-client-secret-file ROOT_ONLY_FILE] \
    --apply

Configures Agent Room behind a co-located Caddy instance. The application
serves HTTP on 127.0.0.1:8443; Caddy owns public TLS. PostgreSQL credentials
are generated, systemd credentials are encrypted, and pgBackRest is
initialized. If no OIDC secret file is supplied, the secret is read without
echo from the controlling terminal. This script never edits Caddy's config.
EOF
}

public_url=""
oidc_issuer=""
oidc_client_id=""
oidc_client_secret_file=""
apply=0
while (($#)); do
  case "$1" in
    --public-url) public_url="${2:?}"; shift 2 ;;
    --oidc-issuer) oidc_issuer="${2:?}"; shift 2 ;;
    --oidc-client-id) oidc_client_id="${2:?}"; shift 2 ;;
    --oidc-client-secret-file) oidc_client_secret_file="${2:?}"; shift 2 ;;
    --apply) apply=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) usage; exit 2 ;;
  esac
done

((apply == 1)) || { usage; exit 2; }
require_root
[[ "${public_url}" =~ ^https://[A-Za-z0-9.-]+(:[0-9]+)?$ ]] ||
  die "public URL must be an HTTPS origin without a path, query, or fragment"
[[ "${oidc_issuer}" =~ ^https://[A-Za-z0-9.-]+(:[0-9]+)?(/[-A-Za-z0-9._~%/]*)?$ ]] ||
  die "OIDC issuer must be an HTTPS URL without credentials, query, or fragment"
[[ "${oidc_client_id}" =~ ^[A-Za-z0-9][A-Za-z0-9._:-]{0,254}$ ]] ||
  die "OIDC client ID contains unsupported characters"

for command_name in awk chmod chown cmp createdb getent grep install mktemp openssl \
  pgbackrest psql rm runuser stat systemctl systemd-creds; do
  require_command "${command_name}"
done
getent passwd "${AGENTROOM_USER}" >/dev/null ||
  die "Agent Room service user is absent; run install-host.sh first"
credentials_dir="${AGENTROOM_CONFIG_DIR}/credentials"
install -d -o root -g "${AGENTROOM_GROUP}" -m 0750 "${AGENTROOM_CONFIG_DIR}"
install -d -o root -g root -m 0700 "${credentials_dir}"
install -d -o "${AGENTROOM_USER}" -g "${AGENTROOM_GROUP}" -m 0700 \
  "${AGENTROOM_STATE_DIR}/artifacts" "${AGENTROOM_STATE_DIR}/workspaces"

encrypt_value() {
  local credential_name="$1"
  local value="$2"
  local destination="${credentials_dir}/${credential_name}.cred"
  [[ ! -e "${destination}" ]] ||
    die "refusing to replace existing encrypted credential: ${destination}"
  printf '%s' "${value}" |
    systemd-creds encrypt --name="${credential_name}" - "${destination}"
  chown root:root "${destination}"
  chmod 0600 "${destination}"
}

database_credential="${credentials_dir}/database-url.cred"
if [[ ! -e "${database_credential}" ]]; then
  database_password="$(openssl rand -hex 32)"
  if [[ "$(runuser -u postgres -- psql -Atqc \
    "SELECT 1 FROM pg_roles WHERE rolname='${DATABASE_USER}'")" == "1" ]]; then
    runuser -u postgres -- psql -v ON_ERROR_STOP=1 -c \
      "ALTER ROLE ${DATABASE_USER} WITH LOGIN PASSWORD '${database_password}'" >/dev/null
  else
    runuser -u postgres -- psql -v ON_ERROR_STOP=1 -c \
      "CREATE ROLE ${DATABASE_USER} WITH LOGIN PASSWORD '${database_password}'" >/dev/null
  fi
  if [[ "$(runuser -u postgres -- psql -Atqc \
    "SELECT 1 FROM pg_database WHERE datname='${DATABASE_NAME}'")" != "1" ]]; then
    runuser -u postgres -- createdb --owner="${DATABASE_USER}" "${DATABASE_NAME}"
  fi
  encrypt_value database-url \
    "postgresql://${DATABASE_USER}:${database_password}@127.0.0.1:5432/${DATABASE_NAME}?sslmode=disable"
  unset database_password
  log "created PostgreSQL role, database, and encrypted application credential"
else
  log "preserving existing encrypted database credential"
fi

session_credential="${credentials_dir}/session-secret.cred"
if [[ ! -e "${session_credential}" ]]; then
  session_secret="$(openssl rand -base64 48)"
  encrypt_value session-secret "${session_secret}"
  unset session_secret
  log "created encrypted session credential"
else
  log "preserving existing encrypted session credential"
fi

oidc_credential="${credentials_dir}/oidc-client-secret.cred"
if [[ ! -e "${oidc_credential}" ]]; then
  if [[ -n "${oidc_client_secret_file}" ]]; then
    [[ -f "${oidc_client_secret_file}" && ! -L "${oidc_client_secret_file}" ]] ||
      die "OIDC client secret must be a regular, non-symlink file"
    secret_mode="$(stat -c '%a' "${oidc_client_secret_file}")"
    (( (8#${secret_mode} & 8#077) == 0 )) ||
      die "OIDC client secret file must not be accessible by group or other"
    oidc_client_secret="$(<"${oidc_client_secret_file}")"
  else
    [[ -r /dev/tty ]] || die "a controlling terminal or --oidc-client-secret-file is required"
    printf 'OIDC client secret: ' >/dev/tty
    IFS= read -r -s oidc_client_secret </dev/tty
    printf '\n' >/dev/tty
  fi
  [[ -n "${oidc_client_secret}" ]] || die "OIDC client secret must not be empty"
  encrypt_value oidc-client-secret "${oidc_client_secret}"
  unset oidc_client_secret
  log "created encrypted OIDC client credential"
else
  [[ -z "${oidc_client_secret_file}" ]] ||
    die "refusing to ignore a supplied OIDC secret while an encrypted credential exists"
  if [[ -r "${AGENTROOM_CONFIG_FILE}" ]]; then
    existing_oidc_client_id="$(awk -F= \
      '$1 == "AGENTROOM_OIDC_CLIENT_ID" { print substr($0, index($0, "=") + 1) }' \
      "${AGENTROOM_CONFIG_FILE}")"
    [[ -z "${existing_oidc_client_id}" || "${existing_oidc_client_id}" == "${oidc_client_id}" ]] ||
      die "OIDC client ID changed; rotate the encrypted OIDC credential explicitly"
  fi
  log "preserving existing encrypted OIDC client credential"
fi

config_temporary="$(mktemp "${AGENTROOM_CONFIG_FILE}.tmp.XXXXXX")"
{
  printf '%s\n' \
    '# Strict dotenv syntax. This file contains nonsecrets only.' \
    'AGENTROOM_HTTP_ADDR=127.0.0.1:8443' \
    'AGENTROOM_ADMIN_ADDR=127.0.0.1:9090' \
    'AGENTROOM_ADAPTER_ADDR=127.0.0.1:9091' \
    "AGENTROOM_PUBLIC_URL=${public_url}" \
    "AGENTROOM_ALLOWED_ORIGINS=${public_url}" \
    'AGENTROOM_TRUSTED_PROXY_CIDRS=127.0.0.1/32' \
    "AGENTROOM_OIDC_ISSUER=${oidc_issuer}" \
    "AGENTROOM_OIDC_CLIENT_ID=${oidc_client_id}" \
    "AGENTROOM_OIDC_REDIRECT_URL=${public_url}/api/v1/auth/callback" \
    'AGENTROOM_ARTIFACT_DIR=/var/lib/agentroom/artifacts' \
    'AGENTROOM_WEB_DIR=/opt/agentroom/current/web' \
    'AGENTROOM_LOG_LEVEL=info' \
    'AGENTROOM_OTEL_EXPORTER_OTLP_ENDPOINT=' \
    'AGENTROOM_MAX_REQUEST_BYTES=262144' \
    'AGENTROOM_SHUTDOWN_TIMEOUT=30s'
} >"${config_temporary}"
install -o root -g "${AGENTROOM_GROUP}" -m 0640 \
  "${config_temporary}" "${AGENTROOM_CONFIG_FILE}"
rm -f -- "${config_temporary}"

log "wrote production config"

pgbackrest_config=/etc/pgbackrest/pgbackrest.conf
postgres_archive_config="/etc/postgresql/${POSTGRESQL_MAJOR}/main/conf.d/agentroom-archive.conf"
install -d -o root -g postgres -m 0750 /etc/pgbackrest
install -d -o postgres -g postgres -m 0750 \
  /var/backups/pgbackrest /var/log/pgbackrest /var/spool/pgbackrest
if [[ ! -e "${pgbackrest_config}" ]]; then
  install -o root -g postgres -m 0640 \
    "${script_dir}/pgbackrest/pgbackrest.conf.example" "${pgbackrest_config}"
elif ! grep -q '^\[agentroom\]$' "${pgbackrest_config}"; then
  die "existing pgBackRest config does not define the agentroom stanza"
fi
if [[ -e "${postgres_archive_config}" ]] &&
   ! cmp -s "${script_dir}/pgbackrest/postgresql.conf.snippet" "${postgres_archive_config}"; then
  die "existing Agent Room PostgreSQL archive config differs from the reviewed template"
fi
install -o root -g postgres -m 0640 \
  "${script_dir}/pgbackrest/postgresql.conf.snippet" "${postgres_archive_config}"
systemctl restart postgresql
systemctl is-active --quiet postgresql ||
  die "PostgreSQL did not become active after enabling archive settings"
runuser -u postgres -- pgbackrest --stanza=agentroom stanza-create
runuser -u postgres -- pgbackrest --stanza=agentroom check

log "instance configuration complete"
log "deploy a signed release, then point co-located Caddy at 127.0.0.1:8443"
