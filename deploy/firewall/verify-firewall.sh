#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'

ingress_ip=""
while (($#)); do
  case "$1" in
    --ingress-ip) ingress_ip="${2:?}"; shift 2 ;;
    -h|--help) printf 'Usage: verify-firewall.sh --ingress-ip IPV4\n'; exit 0 ;;
    *) printf 'Unknown argument\n' >&2; exit 2 ;;
  esac
done

[[ "${ingress_ip}" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] || {
  printf 'A literal ingress IPv4 address is required\n' >&2
  exit 1
}
command -v nft >/dev/null 2>&1 || { printf 'nft is required\n' >&2; exit 1; }
command -v ss >/dev/null 2>&1 || { printf 'ss is required\n' >&2; exit 1; }

rules="$(nft -j list table inet agentroom)"
jq -e --arg source "${ingress_ip}" '
  [.nftables[].rule? |
    select(.chain == "agentroom-mtls-only") |
    .expr] |
  any(tostring | contains($source))
' <<<"${rules}" >/dev/null || {
  printf 'No 8443 allow rule for ingress host %s\n' "${ingress_ip}" >&2
  exit 1
}
jq -e '
  [.nftables[].rule? |
    select(.chain == "agentroom-mtls-only") |
    .expr] |
  any(tostring | contains("drop"))
' <<<"${rules}" >/dev/null || {
  printf 'No default 8443 drop rule found\n' >&2
  exit 1
}

if ss -H -ltn 'sport = :9090' | awk '{print $4}' |
  grep -Ev '^(127\.0\.0\.1|\[::1\]):9090$' | grep -q .; then
  printf 'Admin listener 9090 is exposed beyond loopback\n' >&2
  exit 1
fi
if ss -H -ltn 'sport = :9091' | awk '{print $4}' |
  grep -Ev '^(127\.0\.0\.1|\[::1\]):9091$' | grep -q .; then
  printf 'Adapter listener 9091 is exposed beyond loopback\n' >&2
  exit 1
fi
ss -H -ltn 'sport = :9091' | grep -q ':9091' || {
  printf 'Expected loopback adapter listener 9091 is absent\n' >&2
  exit 1
}
ss -H -ltn 'sport = :8443' | grep -q ':8443' || {
  printf 'Expected Agent Room 8443 listener is absent\n' >&2
  exit 1
}
printf 'Agent Room listener and nftables restrictions verified\n'
