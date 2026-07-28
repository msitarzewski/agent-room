#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'

usage() {
  printf 'Usage: build-release.sh --version VERSION --output-dir DIR --cosign-key KEY\n' >&2
}

version=""
output_dir=""
cosign_key=""
while (($#)); do
  case "$1" in
    --version) version="${2:?}"; shift 2 ;;
    --output-dir) output_dir="${2:?}"; shift 2 ;;
    --cosign-key) cosign_key="${2:?}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) usage; exit 2 ;;
  esac
done
[[ -n "${version}" && -n "${output_dir}" && -f "${cosign_key}" ]] || { usage; exit 2; }

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "${repo_root}"
for command_name in git go jq npm; do
  command -v "${command_name}" >/dev/null 2>&1 || {
    printf 'Required release build tool unavailable: %s\n' "${command_name}" >&2
    exit 1
  }
done
[[ -z "$(git status --porcelain --untracked-files=all)" ]] || {
  printf 'Production releases require a clean committed worktree\n' >&2
  exit 1
}
source_revision="$(git rev-parse --verify HEAD)"
[[ "${source_revision}" =~ ^[0-9a-f]{40}$ ]] || {
  printf 'Unable to resolve the exact source commit\n' >&2
  exit 1
}
source_date_epoch="$(git show -s --format=%ct "${source_revision}")"
[[ "${source_date_epoch}" =~ ^[0-9]+$ ]] || {
  printf 'Unable to resolve the source commit timestamp\n' >&2
  exit 1
}

temporary="$(mktemp -d)"
trap 'rm -rf -- "${temporary}"' EXIT
mkdir -p "${temporary}/bin" "${temporary}/web"

npm --prefix web ci --ignore-scripts
npm --prefix web run typecheck
npm --prefix web run lint
npm --prefix web run api:lint -- --extends recommended-strict
npm --prefix web test
npm --prefix web run build
: "${AGENTROOM_TEST_DATABASE_URL:?AGENTROOM_TEST_DATABASE_URL is required for the production release gate}"
tests/security/backend-quality.sh

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -buildvcs=true -o "${temporary}/bin/agentroomd" ./cmd/agentroomd
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -buildvcs=true -o "${temporary}/bin/agentroomctl" ./cmd/agentroomctl
cp -a web/dist/. "${temporary}/web/"
jq -n \
  --arg source_revision "${source_revision}" \
  --argjson source_date_epoch "${source_date_epoch}" \
  '{schema:1,source_revision:$source_revision,source_date_epoch:$source_date_epoch}' \
  >"${temporary}/web/.agentroom-build.json"

[[ -z "$(git status --porcelain --untracked-files=all)" ]] || {
  printf 'Tracked or untracked source changed during the production build\n' >&2
  exit 1
}
SOURCE_REVISION="${source_revision}" SOURCE_DATE_EPOCH="${source_date_epoch}" \
  deploy/package-release.sh \
    --version "${version}" \
    --agentroomd "${temporary}/bin/agentroomd" \
    --agentroomctl "${temporary}/bin/agentroomctl" \
    --web-dir "${temporary}/web" \
    --output-dir "${output_dir}" \
    --cosign-key "${cosign_key}"
