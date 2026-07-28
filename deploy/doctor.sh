#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)/lib/common.sh"

require_root
for command_name in aa-status curl getent jq nft openssl ss systemctl systemd-analyze; do
  require_command "${command_name}"
done

failures=0
check() {
  local description="$1"
  shift
  if "$@"; then
    printf 'PASS: %s\n' "${description}"
  else
    printf 'FAIL: %s\n' "${description}" >&2
    failures=$((failures + 1))
  fi
}

check "agentroom user exists" getent passwd "${AGENTROOM_USER}"
check "current release symlink is valid" test -x "${AGENTROOM_CURRENT_LINK}/bin/agentroomd"
check "production config exists" test -r "${AGENTROOM_CONFIG_FILE}"
check "config contains no .invalid placeholders" bash -c "! grep -q '\\.invalid' '$AGENTROOM_CONFIG_FILE'"
check "trusted proxy is explicitly configured without TEST-NET placeholders" bash -c \
  "grep -Eq '^AGENTROOM_TRUSTED_PROXY_CIDRS=.+/[0-9]+(,.+(/[0-9]+))?$' '$AGENTROOM_CONFIG_FILE' &&
   ! grep -Eq '^AGENTROOM_TRUSTED_PROXY_CIDRS=.*192\\.0\\.2\\.' '$AGENTROOM_CONFIG_FILE'"
check "systemd unit is enabled" systemctl is-enabled --quiet "${AGENTROOM_UNIT}"
check "service is active" systemctl is-active --quiet "${AGENTROOM_UNIT}"
check "service runs as agentroom" bash -c \
  "[[ \"\$(systemctl show -p User --value '$AGENTROOM_UNIT')\" == '$AGENTROOM_USER' ]]"
check "AppArmor daemon profile is enforced" grep -q '^agentroomd (enforce)' \
  /sys/kernel/security/apparmor/profiles
check "AppArmor CLI profile is enforced" grep -q '^agentroomctl (enforce)' \
  /sys/kernel/security/apparmor/profiles
check "nftables has an Agent Room 8443 restriction" bash -c \
  "nft list ruleset | grep -q 'agentroom-mtls-only'"
check "admin listener is loopback-only" bash -c \
  "ss -H -ltn 'sport = :9090' | awk '{print \$4}' | grep -Eq '^(127\\.0\\.0\\.1|\\[::1\\]):9090$'"
check "adapter listener is loopback-only" bash -c \
  "ss -H -ltn 'sport = :9091' | awk '{print \$4}' | grep -Eq '^(127\\.0\\.0\\.1|\\[::1\\]):9091$'"
check "mTLS listener is present" bash -c "ss -H -ltn 'sport = :8443' | grep -q ':8443'"
check "private liveness passes" curl --fail --silent --show-error --max-time 5 http://127.0.0.1:9090/livez
check "private readiness passes" curl --fail --silent --show-error --max-time 5 http://127.0.0.1:9090/readyz
check "application doctor passes" "${AGENTROOM_CURRENT_LINK}/bin/agentroomctl" \
  --config "${AGENTROOM_CONFIG_FILE}" doctor

exposure="$(systemd-analyze security --no-pager --json=short "${AGENTROOM_UNIT}" |
  jq -er '.[0].exposure')"
if awk -v value="${exposure}" 'BEGIN { exit !(value <= 2.0) }'; then
  printf 'PASS: systemd exposure score %s <= 2.0\n' "${exposure}"
else
  printf 'FAIL: systemd exposure score %s exceeds 2.0\n' "${exposure}" >&2
  failures=$((failures + 1))
fi

for credential in database-url.cred session-secret.cred oidc-client-secret.cred tls-key.cred; do
  path="${AGENTROOM_CONFIG_DIR}/credentials/${credential}"
  check "${credential} exists and is root-only" bash -c \
    "[[ -f '$path' && \"\$(stat -c '%U:%G:%a' '$path')\" == 'root:root:600' ]]"
done
for public_credential in tls-cert.pem tls-client-ca.pem; do
  path="${AGENTROOM_CONFIG_DIR}/credentials/${public_credential}"
  mode="$(stat -c '%a' "${path}" 2>/dev/null || printf '777')"
  if [[ -f "${path}" ]] && (( (8#${mode} & 8#022) == 0 )); then
    printf 'PASS: %s exists and is not writable by group/other\n' "${public_credential}"
  else
    printf 'FAIL: %s is absent or writable by group/other\n' "${public_credential}" >&2
    failures=$((failures + 1))
  fi
done
check "server certificate remains valid for 30 days" openssl x509 -checkend 2592000 -noout \
  -in "${AGENTROOM_CONFIG_DIR}/credentials/tls-cert.pem"

((failures == 0)) || die "${failures} production doctor checks failed"
printf 'All production doctor checks passed\n'
