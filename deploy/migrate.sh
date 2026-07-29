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
    run_agentroomctl_transient "agentroom-migration-check-$$" migrate "${action}"
    ;;
esac
