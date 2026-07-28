#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
cd "${repo_root}"

for command_name in diff go-licenses node; do
  command -v "${command_name}" >/dev/null 2>&1 || {
    printf 'Required license-audit tool unavailable: %s\n' "${command_name}" >&2
    exit 1
  }
done

for required_path in \
  LICENSE \
  NOTICE \
  SECURITY.md \
  CONTRIBUTING.md \
  THIRD_PARTY_NOTICES.md \
  third_party/go-licenses \
  third_party/web-licenses/react-family-LICENSE; do
  [[ -e "${required_path}" ]] || {
    printf 'Required open-source material is missing: %s\n' "${required_path}" >&2
    exit 1
  }
done

grep -q 'Apache License' LICENSE
grep -q 'Copyright 2026 Michael Sitarzewski' NOTICE
grep -q 'Copyright 2014 CoreOS, Inc' NOTICE
grep -q 'identifier: Apache-2.0' api/openapi/agent-room.v1.yaml

temporary="$(mktemp -d)"
trap 'rm -rf -- "${temporary}"' EXIT
report="${temporary}/go-license-report.csv"
GOOS=linux GOARCH=amd64 \
  go-licenses report ./cmd/agentroomd ./cmd/agentroomctl \
  --ignore github.com/msitarzewski/agent-room >"${report}"

unexpected="$(
  awk -F, '
    NF >= 3 && $3 != "Apache-2.0" && $3 != "BSD-2-Clause" &&
    $3 != "BSD-3-Clause" && $3 != "ISC" && $3 != "MIT" {
      print
    }
  ' "${report}"
)"
if [[ -n "${unexpected}" ]]; then
  printf 'Disallowed or unknown Go runtime license:\n%s\n' "${unexpected}" >&2
  exit 1
fi

GOOS=linux GOARCH=amd64 \
  go-licenses save ./cmd/agentroomd ./cmd/agentroomctl \
  --ignore github.com/msitarzewski/agent-room \
  --save_path "${temporary}/go-licenses"
diff -ruNB third_party/go-licenses "${temporary}/go-licenses"

node <<'NODE'
const lock = require("./web/package-lock.json");
const allowed = new Set(["Apache-2.0", "BSD-2-Clause", "BSD-3-Clause", "ISC", "MIT"]);
const unexpected = [];
for (const [path, metadata] of Object.entries(lock.packages ?? {})) {
  if (!path.startsWith("node_modules/") || metadata.dev === true) continue;
  if (!allowed.has(metadata.license)) {
    unexpected.push(`${path.slice("node_modules/".length)}: ${metadata.license ?? "UNKNOWN"}`);
  }
}
if (unexpected.length) {
  console.error(`Disallowed or unknown browser runtime license:\n${unexpected.join("\n")}`);
  process.exit(1);
}
NODE

printf 'Runtime licenses and redistributed license texts verified\n'
