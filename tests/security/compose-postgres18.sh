#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
compose_file="${repo_root}/deploy/compose/compose.yaml"
command -v docker >/dev/null 2>&1 || { printf 'docker is required\n' >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { printf 'jq is required\n' >&2; exit 1; }
command -v curl >/dev/null 2>&1 || { printf 'curl is required\n' >&2; exit 1; }

temporary="$(mktemp -d)"
project_name="agentroom-pg18-$PPID-$$"
cleanup() {
  docker compose --project-name "${project_name}" --file "${compose_file}" down \
    --volumes --remove-orphans >/dev/null 2>&1 || true
  rm -rf -- "${temporary}"
}
trap cleanup EXIT

install -m 0600 /dev/null "${temporary}/postgres-password"
printf 'agentroom-compose-regression\n' >"${temporary}/postgres-password"
for secret in database-url session-secret oidc-client-secret; do
  install -m 0600 /dev/null "${temporary}/${secret}"
  printf 'compose-regression-only\n' >"${temporary}/${secret}"
done
export AGENTROOM_DEV_SECRETS_DIR="${temporary}"
export AGENTROOM_IMAGE="agentroom:compose-regression-not-started"
export AGENTROOM_DEV_POSTGRES_DB="agentroom_test"
# Use high, per-run host ports so this isolated regression does not collide with
# a developer's long-running Agent Room stack. Containers still listen on their
# fixed internal ports.
export AGENTROOM_DEV_POSTGRES_PORT="$((49152 + (RANDOM % 8000)))"
export AGENTROOM_DEV_OIDC_PORT="$((57344 + (RANDOM % 8000)))"
rendered="$(docker compose --project-name "${project_name}" --file "${compose_file}" config --format json)"
jq -e '
  any(.services.postgres.volumes[]; .target == "/var/lib/postgresql") and
  (any(.services.postgres.volumes[]; .target == "/var/lib/postgresql/data") | not) and
  (.services.postgres.image | test("@sha256:[0-9a-f]{64}$")) and
  (.services.dex.image | test("@sha256:[0-9a-f]{64}$"))
' <<<"${rendered}" >/dev/null || {
  printf 'Compose images must be digest-pinned and PostgreSQL 18 must use /var/lib/postgresql\n' >&2
  exit 1
}

docker compose --project-name "${project_name}" --file "${compose_file}" up \
  --detach --wait --wait-timeout 90 postgres dex
discovery="$(curl --fail --silent --show-error --max-time 10 \
  "http://127.0.0.1:${AGENTROOM_DEV_OIDC_PORT}/dex/.well-known/openid-configuration")"
jq -e --arg issuer "http://127.0.0.1:${AGENTROOM_DEV_OIDC_PORT}/dex" '.issuer == $issuer and
  (.authorization_endpoint | type == "string") and
  (.token_endpoint | type == "string") and
  (.jwks_uri | type == "string")' <<<"${discovery}" >/dev/null || {
  printf 'Dex discovery document is incomplete or has the wrong issuer\n' >&2
  exit 1
}
container_discovery="$(docker run --rm --network host \
  --entrypoint /bin/sh \
  ghcr.io/dexidp/dex@sha256:8499afd690c437f52301efd2b05b2455da5bd2dfc20332cd697dc9937f808462 \
  -c "wget --quiet --output-document=- http://127.0.0.1:${AGENTROOM_DEV_OIDC_PORT}/dex/.well-known/openid-configuration")"
jq -e --arg issuer "http://127.0.0.1:${AGENTROOM_DEV_OIDC_PORT}/dex" \
  '.issuer == $issuer' <<<"${container_discovery}" >/dev/null || {
  printf 'Host-network application containers cannot resolve the canonical Dex issuer\n' >&2
  exit 1
}
docker compose --project-name "${project_name}" --file "${compose_file}" exec \
  --no-TTY postgres psql --username agentroom --dbname "${AGENTROOM_DEV_POSTGRES_DB}" \
  --set ON_ERROR_STOP=1 \
  --command 'CREATE TABLE compose_persistence(value text PRIMARY KEY);' \
  --command "INSERT INTO compose_persistence(value) VALUES ('survives-restart');"
AGENTROOM_TEST_DATABASE_URL="postgres://agentroom:agentroom-compose-regression@127.0.0.1:${AGENTROOM_DEV_POSTGRES_PORT}/${AGENTROOM_DEV_POSTGRES_DB}?sslmode=disable" \
  go test -race -tags=integration ./internal/postgres ./tests/api
docker compose --project-name "${project_name}" --file "${compose_file}" restart postgres
docker compose --project-name "${project_name}" --file "${compose_file}" up \
  --detach --wait --wait-timeout 90 postgres
value="$(docker compose --project-name "${project_name}" --file "${compose_file}" exec \
  --no-TTY postgres psql --username agentroom --dbname "${AGENTROOM_DEV_POSTGRES_DB}" \
  --tuples-only --no-align --command 'SELECT value FROM compose_persistence;')"
[[ "${value}" == "survives-restart" ]] || {
  printf 'PostgreSQL data did not survive a container restart\n' >&2
  exit 1
}

printf 'PostgreSQL 18 persistence and local OIDC discovery passed\n'
