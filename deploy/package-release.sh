#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"

usage() {
  cat >&2 <<'USAGE'
Usage:
  package-release.sh --version VERSION --agentroomd PATH --agentroomctl PATH \
    --web-dir PATH --output-dir PATH --cosign-key PATH

Development-only unsigned package:
  package-release.sh ... --development-unsigned
USAGE
}

version=""
daemon_path=""
ctl_path=""
web_dir=""
output_dir=""
cosign_key=""
development_unsigned=0

while (($#)); do
  case "$1" in
    --version) version="${2:?}"; shift 2 ;;
    --agentroomd) daemon_path="${2:?}"; shift 2 ;;
    --agentroomctl) ctl_path="${2:?}"; shift 2 ;;
    --web-dir) web_dir="${2:?}"; shift 2 ;;
    --output-dir) output_dir="${2:?}"; shift 2 ;;
    --cosign-key) cosign_key="${2:?}"; shift 2 ;;
    --development-unsigned) development_unsigned=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) usage; printf 'Unknown argument: %s\n' "$1" >&2; exit 2 ;;
  esac
done

[[ -n "${version}" && "${version}" =~ ^[0-9A-Za-z][0-9A-Za-z._+-]{0,127}$ ]] ||
  { usage; printf 'A safe --version is required\n' >&2; exit 2; }
for legal_path in LICENSE NOTICE THIRD_PARTY_NOTICES.md third_party; do
  [[ -e "${repo_root}/${legal_path}" ]] || {
    printf 'Required license material is missing: %s\n' "${legal_path}" >&2
    exit 1
  }
done
[[ -f "${daemon_path}" && -x "${daemon_path}" ]] || { printf 'agentroomd must be executable\n' >&2; exit 1; }
[[ -f "${ctl_path}" && -x "${ctl_path}" ]] || { printf 'agentroomctl must be executable\n' >&2; exit 1; }
[[ -d "${web_dir}" && -f "${web_dir}/index.html" ]] || { printf 'built web directory with index.html is required\n' >&2; exit 1; }
[[ -z "$(find "${web_dir}" -type l -print -quit)" ]] || {
  printf 'Built web directory may not contain symlinks\n' >&2
  exit 1
}
[[ -n "${output_dir}" ]] || { usage; exit 2; }
epoch="${SOURCE_DATE_EPOCH:-}"
if [[ -z "${epoch}" ]]; then
  if ((development_unsigned == 0)); then
    printf 'SOURCE_DATE_EPOCH is required for production release packaging\n' >&2
    exit 1
  fi
  epoch=0
fi
[[ "${epoch}" =~ ^[0-9]+$ ]] || {
  printf 'SOURCE_DATE_EPOCH must be a non-negative integer\n' >&2
  exit 1
}
for command_name in file find gzip jq sha256sum syft tar; do
  command -v "${command_name}" >/dev/null 2>&1 || {
    printf 'Required release tool unavailable: %s\n' "${command_name}" >&2
    exit 1
  }
done
built_at="$(jq -nr --argjson epoch "${epoch}" '$epoch | todateiso8601')" || {
  printf 'SOURCE_DATE_EPOCH is outside the supported timestamp range\n' >&2
  exit 1
}

tar --version | grep -q 'GNU tar' || {
  printf 'GNU tar is required for deterministic linux/amd64 release archives\n' >&2
  exit 1
}

file "${daemon_path}" | grep -Eq 'ELF 64-bit.*x86-64' || {
  printf 'agentroomd is not a linux/amd64 ELF binary\n' >&2
  exit 1
}
file "${ctl_path}" | grep -Eq 'ELF 64-bit.*x86-64' || {
  printf 'agentroomctl is not a linux/amd64 ELF binary\n' >&2
  exit 1
}

if ((development_unsigned == 0)); then
  command -v go >/dev/null 2>&1 || { printf 'go is required to verify binary provenance\n' >&2; exit 1; }
  [[ "${SOURCE_REVISION:-}" =~ ^[0-9a-f]{40}$ ]] || {
    printf 'SOURCE_REVISION must be the exact 40-character Git commit for production releases\n' >&2
    exit 1
  }
  for binary_path in "${daemon_path}" "${ctl_path}"; do
    binary_revision="$(go version -m -json "${binary_path}" |
      jq -r '.Settings[]? | select(.Key == "vcs.revision") | .Value' | head -n 1)"
    binary_modified="$(go version -m -json "${binary_path}" |
      jq -r '.Settings[]? | select(.Key == "vcs.modified") | .Value' | head -n 1)"
    [[ "${binary_revision}" == "${SOURCE_REVISION}" && "${binary_modified}" == "false" ]] || {
      printf 'Binary provenance does not match clean SOURCE_REVISION: %s\n' "${binary_path}" >&2
      exit 1
    }
  done
  web_provenance="${web_dir}/.agentroom-build.json"
  jq -e \
    --arg revision "${SOURCE_REVISION}" \
    --argjson epoch "${epoch}" \
    '.schema == 1 and .source_revision == $revision and .source_date_epoch == $epoch' \
    "${web_provenance}" >/dev/null 2>&1 || {
      printf 'Web build provenance does not match SOURCE_REVISION and SOURCE_DATE_EPOCH\n' >&2
      exit 1
    }
  command -v cosign >/dev/null 2>&1 || { printf 'cosign is required\n' >&2; exit 1; }
  [[ -f "${cosign_key}" ]] || { printf 'A readable --cosign-key is required\n' >&2; exit 1; }
fi

mkdir -p "${output_dir}"
stage="$(mktemp -d)"
sbom_temporary="${stage}.sbom.spdx.json"
trap 'rm -rf -- "${stage}"; rm -f -- "${sbom_temporary}"' EXIT
mkdir -p "${stage}/bin" "${stage}/web"
install -m 0755 "${daemon_path}" "${stage}/bin/agentroomd"
install -m 0755 "${ctl_path}" "${stage}/bin/agentroomctl"
install -m 0644 "${repo_root}/LICENSE" "${stage}/LICENSE"
install -m 0644 "${repo_root}/NOTICE" "${stage}/NOTICE"
install -m 0644 "${repo_root}/THIRD_PARTY_NOTICES.md" "${stage}/THIRD_PARTY_NOTICES.md"
cp -a "${repo_root}/third_party" "${stage}/third_party"
find "${stage}/third_party" -type f -exec chmod 0644 {} +
find "${stage}/third_party" -type d -exec chmod 0755 {} +
cp -a "${web_dir}/." "${stage}/web/"
find "${stage}/web" -type f -exec chmod 0644 {} +
find "${stage}/web" -type d -exec chmod 0755 {} +

SOURCE_DATE_EPOCH="${epoch}" syft scan "dir:${stage}" \
  --source-name "agentroom-${version}" -o "spdx-json=${sbom_temporary}"
jq -S \
  --arg name "agentroom-${version}" \
  --arg namespace "https://agentroom.dev/spdx/releases/${version}/${SOURCE_REVISION:-unknown}" \
  --arg built_at "${built_at}" \
  '.name=$name
   | .documentNamespace=$namespace
   | .creationInfo.created=$built_at
   | if .packages then .packages |= sort_by(.SPDXID) else . end
   | if .files then .files |= sort_by(.SPDXID) else . end
   | if .relationships then .relationships |= sort_by(.spdxElementId,.relationshipType,.relatedSpdxElement) else . end' \
  "${sbom_temporary}" >"${stage}/sbom.spdx.json"
[[ -s "${stage}/sbom.spdx.json" ]] || { printf 'SBOM generation failed\n' >&2; exit 1; }

files_json="${stage}/.files.jsonl"
: >"${files_json}"
while IFS= read -r -d '' file_path; do
  relative="${file_path#"${stage}/"}"
  [[ "${relative}" =~ ^[A-Za-z0-9][A-Za-z0-9._/+@-]*$ ]] || {
    printf 'Release path contains unsupported characters: %q\n' "${relative}" >&2
    exit 1
  }
  digest="$(sha256sum "${file_path}" | awk '{print $1}')"
  size="$(wc -c <"${file_path}" | tr -d ' ')"
  jq -cn --arg path "${relative}" --arg sha256 "${digest}" --argjson size "${size}" \
    '{path:$path,sha256:$sha256,size:$size}' >>"${files_json}"
done < <(find "${stage}" -type f ! -name manifest.json ! -name .files.jsonl -print0 | sort -z)

if ((development_unsigned == 0)); then
  signed_json=true
else
  signed_json=false
fi
jq -s \
  --arg version "${version}" \
  --arg target "linux/amd64" \
  --arg source_revision "${SOURCE_REVISION:-unknown}" \
  --arg built_at "${built_at}" \
  --argjson signed "${signed_json}" \
  '{schema:1,version:$version,target:$target,source_revision:$source_revision,built_at:$built_at,signed:$signed,files:.}' \
  "${files_json}" >"${stage}/manifest.json"
rm -f "${files_json}"

artifact="${output_dir%/}/agentroom-${version}-linux-amd64.tar.gz"
tar --sort=name --mtime="@${epoch}" --owner=0 --group=0 --numeric-owner \
  --pax-option=delete=atime,delete=ctime \
  --format=posix -C "${stage}" -cf - . | gzip -n >"${artifact}"

(
  cd "${output_dir}"
  sha256sum "$(basename "${artifact}")" >"$(basename "${artifact}").sha256"
)

if ((development_unsigned == 0)); then
  cosign sign-blob --new-bundle-format=false --use-signing-config=false \
    --yes --key "${cosign_key}" \
    --output-signature "${artifact}.sig" "${artifact}"
  [[ -s "${artifact}.sig" ]] || { printf 'cosign did not create a signature\n' >&2; exit 1; }
else
  printf 'WARNING: created development-only unsigned artifact; production verification will reject it\n' >&2
fi

printf '%s\n' "${artifact}"
