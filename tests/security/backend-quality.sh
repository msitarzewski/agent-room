#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
cd "${repo_root}"
: "${AGENTROOM_TEST_DATABASE_URL:?AGENTROOM_TEST_DATABASE_URL is required for integration tests}"
temporary="$(mktemp -d)"
trap 'rm -rf -- "${temporary}"' EXIT

go vet -tags=integration ./cmd/... ./internal/... ./tests/security ./tests/api
go test -race \
  -covermode=atomic \
  -coverpkg=./internal/... \
  -coverprofile="${temporary}/unit.out" \
  ./internal/... ./tests/security
go test -race -tags=integration \
  -covermode=atomic \
  -coverpkg=./internal/... \
  -coverprofile="${temporary}/auth-integration.out" \
  ./internal/auth
go test -race -tags=integration \
  -covermode=atomic \
  -coverpkg=./internal/... \
  -coverprofile="${temporary}/httpapi-integration.out" \
  ./internal/httpapi
go test -race -tags=integration \
  -covermode=atomic \
  -coverpkg=./internal/... \
  -coverprofile="${temporary}/postgres-integration.out" \
  ./internal/postgres
go test -race -tags=integration \
  -covermode=atomic \
  -coverpkg=./internal/... \
  -coverprofile="${temporary}/api-integration.out" \
  ./tests/api
awk '
  FNR > 1 {
    key = $1 " " $2
    if (!(key in counts) || $3 > counts[key]) {
      counts[key] = $3
    }
  }
  END {
    for (key in counts) {
      print key, counts[key]
    }
  }
' "${temporary}/unit.out" "${temporary}/auth-integration.out" "${temporary}/httpapi-integration.out" "${temporary}/postgres-integration.out" \
  "${temporary}/api-integration.out" | sort >"${temporary}/blocks"
{
  printf 'mode: atomic\n'
  cat "${temporary}/blocks"
} >"${temporary}/overall.out"

profile_coverage() {
  local marker="${1:-}"
  awk -v marker="${marker}" '
    NR > 1 && (marker == "" || index($1, marker)) {
      total += $2
      if ($3 > 0) covered += $2
    }
    END {
      if (total == 0) print "0.0"
      else printf "%.1f\n", 100 * covered / total
    }
  ' "${temporary}/overall.out"
}

failed=0
coverage="$(profile_coverage)"
if ! awk -v coverage="${coverage}" 'BEGIN { exit !(coverage + 0 >= 80) }'; then
  failed=1
fi
printf 'Backend statement coverage: %s%% (minimum 80%%)\n' "${coverage}"

for package in app artifacts auth httpapi runner; do
  package_coverage="$(profile_coverage "/internal/${package}/")"
  if ! awk -v coverage="${package_coverage}" 'BEGIN { exit !(coverage + 0 >= 90) }'; then
    failed=1
  fi
  printf 'Critical package %s coverage: %s%% (minimum 90%%)\n' "${package}" "${package_coverage}"
done
exit "${failed}"
