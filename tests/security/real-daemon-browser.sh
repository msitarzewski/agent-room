#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
cd "${repo_root}"

: "${AGENTROOM_TEST_DATABASE_URL:?AGENTROOM_TEST_DATABASE_URL is required}"
: "${AGENTROOM_E2E_DAEMON_BIN:=${repo_root}/build/e2e/agentroomd}"
: "${AGENTROOM_E2E_CTL_BIN:=${repo_root}/build/e2e/agentroomctl}"
: "${AGENTROOM_E2E_DIAGNOSTICS:=${repo_root}/build/e2e-diagnostics}"

[[ -x "${AGENTROOM_E2E_DAEMON_BIN}" ]] || {
  printf 'Missing daemon binary: %s\n' "${AGENTROOM_E2E_DAEMON_BIN}" >&2
  exit 1
}
[[ -x "${AGENTROOM_E2E_CTL_BIN}" ]] || {
  printf 'Missing control binary: %s\n' "${AGENTROOM_E2E_CTL_BIN}" >&2
  exit 1
}
[[ -f "${repo_root}/web/dist/index.html" ]] || {
  printf 'Production web bundle is missing; run npm run build first\n' >&2
  exit 1
}
command -v docker >/dev/null 2>&1 || {
  printf 'docker is required for the pinned Dex identity provider\n' >&2
  exit 1
}

temporary="$(mktemp -d)"
dex_name="agentroom-e2e-dex-${RANDOM}-$$"
daemon_pid=""
mkdir -p "${AGENTROOM_E2E_DIAGNOSTICS}"

cleanup() {
  status=$?
  if [[ -n "${daemon_pid}" ]]; then
    kill -TERM "${daemon_pid}" 2>/dev/null || true
    wait "${daemon_pid}" 2>/dev/null || true
  fi
  docker logs "${dex_name}" >"${AGENTROOM_E2E_DIAGNOSTICS}/dex.log" 2>&1 || true
  docker rm -f "${dex_name}" >/dev/null 2>&1 || true
  rm -rf -- "${temporary}"
  exit "${status}"
}
trap cleanup EXIT

umask 077
printf '%s\n' "${AGENTROOM_TEST_DATABASE_URL}" >"${temporary}/database-url"
printf '%s\n' 'real-daemon-browser-session-secret-32-bytes-minimum' >"${temporary}/session-secret"
printf '%s\n' 'agentroom-dex-local-only' >"${temporary}/oidc-client-secret"
mkdir -p "${temporary}/artifacts" "${temporary}/workspaces"

export AGENTROOM_DEV=true
export AGENTROOM_HTTPS_ADDR=127.0.0.1:58443
export AGENTROOM_ADMIN_ADDR=127.0.0.1:59090
export AGENTROOM_ADAPTER_ADDR=127.0.0.1:59091
export AGENTROOM_PUBLIC_URL=http://127.0.0.1:58443
export AGENTROOM_ALLOWED_ORIGINS=http://127.0.0.1:58443
export AGENTROOM_OIDC_ISSUER=http://127.0.0.1:5556/dex
export AGENTROOM_OIDC_CLIENT_ID=agentroom-local
export AGENTROOM_OIDC_REDIRECT_URL=http://127.0.0.1:58443/api/v1/auth/callback
export AGENTROOM_ARTIFACT_DIR="${temporary}/artifacts"
export AGENTROOM_WEB_DIR="${repo_root}/web/dist"
export AGENTROOM_WORKSPACE_ROOT="${temporary}/workspaces"
export AGENTROOM_DATABASE_URL_FILE="${temporary}/database-url"
export AGENTROOM_SESSION_SECRET_FILE="${temporary}/session-secret"
export AGENTROOM_OIDC_CLIENT_SECRET_FILE="${temporary}/oidc-client-secret"

dex_image='ghcr.io/dexidp/dex@sha256:8499afd690c437f52301efd2b05b2455da5bd2dfc20332cd697dc9937f808462'
docker run --detach --name "${dex_name}" \
  --publish 127.0.0.1:5556:5556 \
  --read-only --security-opt no-new-privileges --cap-drop ALL \
  --tmpfs /tmp:rw,noexec,nosuid,nodev,size=32m \
  --env DEX_ISSUER=http://127.0.0.1:5556/dex \
  --volume "${repo_root}/deploy/compose/dex-config.yaml:/etc/dex/config.yaml:ro" \
  "${dex_image}" dex serve /etc/dex/config.yaml >/dev/null

for _ in {1..60}; do
  if curl --fail --silent --show-error \
    http://127.0.0.1:5556/dex/.well-known/openid-configuration >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
curl --fail --silent --show-error \
  http://127.0.0.1:5556/dex/.well-known/openid-configuration >/dev/null

"${AGENTROOM_E2E_CTL_BIN}" migrate up
"${AGENTROOM_E2E_CTL_BIN}" bootstrap \
  e2e-project 'Browser E2E' \
  238b6f7b-17bd-4b0d-a195-26e725b776ca \
  operator@agentroom.local

"${AGENTROOM_E2E_DAEMON_BIN}" >"${AGENTROOM_E2E_DIAGNOSTICS}/agentroomd.log" 2>&1 &
daemon_pid=$!
for _ in {1..60}; do
  if curl --fail --silent --show-error http://127.0.0.1:59090/readyz >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "${daemon_pid}" 2>/dev/null; then
    printf 'agentroomd exited before readiness\n' >&2
    exit 1
  fi
  sleep 1
done
curl --fail --silent --show-error http://127.0.0.1:59090/readyz >/dev/null

(
  cd web
  REAL_DAEMON=1 \
    AGENT_ROOM_E2E_BASE_URL=http://127.0.0.1:58443 \
    AGENT_ROOM_E2E_USERNAME=operator@agentroom.local \
    AGENT_ROOM_E2E_PASSWORD=agentroom-local-only \
    npm run test:e2e -- --project=real-daemon-chromium
)
