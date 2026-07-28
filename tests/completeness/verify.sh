#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"

scan_paths=()
for path in cmd internal api db web/src web/tests tests deploy .github Makefile; do
  if [[ -e "$path" ]]; then
    scan_paths+=("$path")
  fi
done

if [[ ${#scan_paths[@]} -eq 0 ]]; then
  echo "completeness: no owned implementation paths found" >&2
  exit 1
fi

failed=0

run_scan() {
  local label="$1"
  local expression="$2"
  shift 2

  local output
  if output="$(
    rg --hidden --line-number --ignore-case \
      --glob '!tests/completeness/verify.sh' \
      --glob '!tests/security/completeness-scan.sh' \
      --glob '!tests/fixtures/**' \
      --glob '!web/package-lock.json' \
      --glob '!**/*.snap' \
      "$expression" "${scan_paths[@]}" "$@" 2>/dev/null
  )"; then
    echo "completeness: ${label} matches found" >&2
    echo "$output" >&2
    failed=1
  fi
}

incomplete_markers='\b(TO''DO|FIX''ME|T''BD|X''XX|HA''CK|ST''UB|unimplemented)\b|not[[:space:]_-]+implemented|placeholder[[:space:]_-]+(response|route|implementation|handler|copy|value)'
skipped_tests='t\.S''kip|test\.s''kip|describe\.s''kip|it\.s''kip|\bx''it\(|\bx''describe\(|test\.o''nly|describe\.o''nly|it\.o''nly'
suppression_markers='no''lint|no''sec|istanbul[[:space:]]+ignore|coverage[[:space:]_-]+ignore'
fake_production_data='\b(mock|fake|sample|demo)(Data|Records|Items)\b|seed[[:space:]_-]+demo'

run_scan "incomplete implementation" "$incomplete_markers"
run_scan "skipped or focused test" "$skipped_tests"
run_scan "unreviewed static-analysis suppression" "$suppression_markers"
run_scan "production-like fabricated data" "$fake_production_data"

if [[ "$failed" -ne 0 ]]; then
  exit 1
fi

echo "completeness: owned implementation scan passed"
