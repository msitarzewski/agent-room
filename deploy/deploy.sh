#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
source "${script_dir}/lib/common.sh"

artifact=""
public_key=""
public_url=""
backup_type="incr"

while (($#)); do
  case "$1" in
    --artifact) artifact="${2:?}"; shift 2 ;;
    --public-key) public_key="${2:?}"; shift 2 ;;
    --public-url) public_url="${2:?}"; shift 2 ;;
    --backup-type) backup_type="${2:?}"; shift 2 ;;
    -h|--help)
      printf 'Usage: deploy.sh --artifact PATH --public-key PATH --public-url HTTPS_ORIGIN [--backup-type TYPE]\n'
      exit 0
      ;;
    *) die "unknown argument: $1" ;;
  esac
done

require_root
for command_name in jq sha256sum systemctl; do require_command "${command_name}"; done
[[ -f "${artifact}" && -f "${public_key}" ]] || die "artifact and public key are required"
[[ "${public_url}" =~ ^https://[^/]+$ ]] || die "public URL must be an HTTPS origin without a path"

mkdir -p "${AGENTROOM_RELEASES_DIR}" "${AGENTROOM_DEPLOYMENT_DIR}"
chmod 0755 "${AGENTROOM_PREFIX}" "${AGENTROOM_RELEASES_DIR}"
chmod 0700 "${AGENTROOM_DEPLOYMENT_DIR}"

verify_extract="$(mktemp -d "${AGENTROOM_RELEASES_DIR}/.verify.XXXXXX")"
cleanup() {
  [[ -z "${verify_extract}" ]] || rm -rf -- "${verify_extract}"
}
trap cleanup EXIT
artifact_sha256="$(sha256sum "${artifact}" | awk '{print $1}')"
version="$("${script_dir}/verify-artifact.sh" \
  --artifact "${artifact}" --public-key "${public_key}" --extract-to "${verify_extract}")"
[[ "$(sha256sum "${artifact}" | awk '{print $1}')" == "${artifact_sha256}" ]] ||
  die "artifact changed during verification"
validate_version "${version}"
release_dir="${AGENTROOM_RELEASES_DIR}/${version}"
[[ ! -e "${release_dir}" ]] || die "release already exists and will not be overwritten: ${version}"

previous=""
if previous="$(current_release_version 2>/dev/null)"; then
  log "current release is ${previous}"
else
  previous=""
fi

switched=0
service_stopped=0
rollback_on_error() {
  local status=$?
  trap - ERR
  if ((service_stopped == 1)); then
    systemctl stop "${AGENTROOM_UNIT}" || true
    if ((switched == 1)); then
      log "deployment failed; restoring previous application pointer"
      if [[ -n "${previous}" ]]; then
        atomic_current_link "${previous}"
      else
        [[ ! -L "${AGENTROOM_CURRENT_LINK}" ]] || unlink "${AGENTROOM_CURRENT_LINK}"
      fi
    fi
    if [[ -L "${AGENTROOM_CURRENT_LINK}" ]]; then
      systemctl start "${AGENTROOM_UNIT}" || true
    fi
  fi
  exit "${status}"
}
trap rollback_on_error ERR

backup_record="${AGENTROOM_DEPLOYMENT_DIR}/${version}.backup.json"
systemctl stop "${AGENTROOM_UNIT}"
service_stopped=1
"${script_dir}/backup.sh" --type "${backup_type}" --output "${backup_record}" \
  --confirm-quiesced AGENTROOM_STOPPED >/dev/null

chown -R root:root "${verify_extract}"
find "${verify_extract}" -type d -exec chmod 0755 {} +
chmod 0755 "${verify_extract}/bin/agentroomd" "${verify_extract}/bin/agentroomctl"
find "${verify_extract}/web" -type f -exec chmod 0644 {} +
chmod 0644 "${verify_extract}/manifest.json" "${verify_extract}/sbom.spdx.json"
mv "${verify_extract}" "${release_dir}"
verify_extract=""

atomic_current_link "${version}"
switched=1
"${script_dir}/migrate.sh" up
systemctl start "${AGENTROOM_UNIT}"
systemctl is-active --quiet "${AGENTROOM_UNIT}" || die "service did not become active"
"${script_dir}/smoke.sh" --public-url "${public_url}"
"${script_dir}/migrate.sh" verify

record_path="${AGENTROOM_DEPLOYMENT_DIR}/${version}.deployment.json"
jq -cn \
  --arg version "${version}" \
  --arg previous "${previous}" \
  --arg artifact_sha256 "${artifact_sha256}" \
  --arg source_revision "$(jq -r '.source_revision' "${release_dir}/manifest.json")" \
  --arg public_url "${public_url}" \
  --arg deployed_at "$(date -u +'%Y-%m-%dT%H:%M:%SZ')" \
  '{schema:1,status:"succeeded",version:$version,previous_version:$previous,
    artifact_sha256:$artifact_sha256,source_revision:$source_revision,
    public_url:$public_url,deployed_at:$deployed_at}' |
  write_json_atomically "${record_path}"
jq -cn --arg version "${version}" '{version:$version}' |
  write_json_atomically "${AGENTROOM_DEPLOYMENT_DIR}/last-successful.json"
if [[ -n "${previous}" ]]; then
  jq -cn --arg version "${previous}" '{version:$version}' |
    write_json_atomically "${AGENTROOM_DEPLOYMENT_DIR}/rollback-target.json"
fi

trap - ERR
service_stopped=0
switched=0
log "deployment ${version} succeeded"
