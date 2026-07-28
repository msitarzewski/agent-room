#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)/lib/common.sh"

action="${1:-up}"
case "${action}" in up|status|verify) ;; *) die "usage: migrate.sh [up|status|verify]" ;; esac
require_root
require_command systemctl
[[ -x "${AGENTROOM_CURRENT_LINK}/bin/agentroomctl" ]] ||
  die "agentroomctl is unavailable through the current release"

case "${action}" in
  up)
    systemctl start "${AGENTROOM_MIGRATE_UNIT}"
    systemctl is-failed --quiet "${AGENTROOM_MIGRATE_UNIT}" &&
      die "migration unit failed"
    ;;
  status|verify)
    systemd-run --quiet --wait --pipe --collect \
      --unit="agentroom-migration-check-$$" \
      --uid="${AGENTROOM_USER}" --gid="${AGENTROOM_GROUP}" \
      --property="Environment=AGENTROOM_CONFIG_FILE=${AGENTROOM_CONFIG_FILE}" \
      --property="LoadCredentialEncrypted=database-url:${AGENTROOM_CONFIG_DIR}/credentials/database-url.cred" \
      --property="Environment=AGENTROOM_DATABASE_URL_FILE=%d/database-url" \
      "${AGENTROOM_CURRENT_LINK}/bin/agentroomctl" \
      --config "${AGENTROOM_CONFIG_FILE}" migrate "${action}"
    ;;
esac
