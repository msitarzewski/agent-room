#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
verifier="${repo_root}/deploy/verify-artifact.sh"
tmp="$(mktemp -d)"
trap 'rm -rf -- "${tmp}"' EXIT

mkdir -p "${tmp}/payload"
printf 'unsigned\n' >"${tmp}/payload/manifest.json"
tar -czf "${tmp}/unsigned.tar.gz" -C "${tmp}/payload" .

if "${verifier}" --artifact "${tmp}/unsigned.tar.gz" --public-key "${tmp}/missing.pub" \
  >"${tmp}/stdout" 2>"${tmp}/stderr"; then
  printf 'Verifier accepted an unsigned artifact\n' >&2
  exit 1
fi
grep -q 'Checksum file not found' "${tmp}/stderr" || {
  printf 'Verifier did not fail for the expected missing integrity evidence\n' >&2
  exit 1
}

mkdir -p "${tmp}/valid/bin" "${tmp}/valid/web" "${tmp}/tools"
printf '#!/usr/bin/env bash\nexit 0\n' >"${tmp}/valid/bin/agentroomd"
printf '#!/usr/bin/env bash\nexit 0\n' >"${tmp}/valid/bin/agentroomctl"
chmod +x "${tmp}/valid/bin/agentroomd" "${tmp}/valid/bin/agentroomctl"
printf '<!doctype html>\n' >"${tmp}/valid/web/index.html"
printf '{"spdxVersion":"SPDX-2.3"}\n' >"${tmp}/valid/sbom.spdx.json"

files_json="${tmp}/files.jsonl"
while IFS= read -r -d '' payload_file; do
  relative="${payload_file#"${tmp}/valid/"}"
  jq -cn \
    --arg path "${relative}" \
    --arg sha256 "$(sha256sum "${payload_file}" | awk '{print $1}')" \
    --argjson size "$(stat -c '%s' "${payload_file}")" \
    '{path: $path, sha256: $sha256, size: $size}' >>"${files_json}"
done < <(find "${tmp}/valid" -type f ! -name manifest.json -print0 | sort -z)
jq -s \
  '{
    schema: 1,
    version: "0.1.0",
    target: "linux/amd64",
    signed: true,
    files: .
  }' "${files_json}" >"${tmp}/valid/manifest.json"

tar -czf "${tmp}/valid.tar.gz" -C "${tmp}/valid" .
(
  cd "${tmp}"
  sha256sum valid.tar.gz >valid.tar.gz.sha256
)
printf 'test signature\n' >"${tmp}/valid.tar.gz.sig"
printf 'test public key\n' >"${tmp}/test.pub"

cat >"${tmp}/tools/cosign" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
cat >"${tmp}/tools/file" <<'EOF'
#!/usr/bin/env bash
printf '%s: ELF 64-bit LSB executable, x86-64\n' "$1"
EOF
chmod +x "${tmp}/tools/cosign" "${tmp}/tools/file"

verification_output="$(
  PATH="${tmp}/tools:${PATH}" "${verifier}" \
    --artifact "${tmp}/valid.tar.gz" \
    --public-key "${tmp}/test.pub"
)"
[[ "${verification_output}" == "0.1.0" ]] || {
  printf 'Verifier stdout must contain only the release version, got: %s\n' \
    "${verification_output}" >&2
  exit 1
}

printf 'Artifact verifier rejected incomplete integrity evidence\n'
printf 'Artifact verifier emitted an exact version contract\n'
