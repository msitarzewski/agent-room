#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
cd "${repo_root}"

failed=0
require_source_pattern() {
  local description="$1"
  local pattern="$2"
  shift 2
  if rg -q "${pattern}" "$@"; then
    printf 'PASS: %s\n' "${description}"
  else
    printf 'FAIL: %s\n' "${description}" >&2
    failed=1
  fi
}

require_source_pattern "adapter address is a distinct runtime setting" \
  'AGENTROOM_ADAPTER_ADDR' internal/config cmd/agentroomd
require_source_pattern "adapter address is loopback validated" \
  'adapter listener must bind to loopback' internal/config
require_source_pattern "public handler explicitly denies adapter paths" \
  'r\.URL\.Path == "/api/v1/ingest".*"/api/v1/mcp"' internal/httpapi/server.go
require_source_pattern "service tokens use a separate adapter handler" \
  'func \(s \*Server\) AdapterHandler\(\)' internal/httpapi/server.go
require_source_pattern "MCP task reads enforce tool capability" \
  'actor\.Can\("task:read"\)' internal/httpapi/mcp.go
require_source_pattern "MCP attention reads enforce tool capability" \
  'actor\.Can\("attention:read"\)' internal/httpapi/mcp.go
require_source_pattern "MCP requests rebind authorization without stateful privilege carryover" \
  'StreamableHTTPOptions\{Stateless: true' internal/httpapi/mcp.go
require_source_pattern "approval consumption shares the destructive action transaction" \
  'consumeApprovalTx\(ctx, tx, cmd\)' internal/postgres/repository.go
require_source_pattern "forwarded addresses require a configured proxy peer" \
  'trustedIP\(peer, trusted\)' internal/httpapi/ratelimit.go
require_source_pattern "public HTTP is restricted to loopback" \
  'HTTP listener must bind to loopback' internal/config/config.go
require_source_pattern "rate-limit buckets are bounded and expire" \
  'len\(l\.buckets\) >= l\.maxKeys' internal/httpapi/ratelimit.go
require_source_pattern "rate-limit LRU eviction removes stored keys" \
  'delete\(l\.buckets, oldest\.Value\.\(string\)\)' internal/httpapi/ratelimit.go
require_source_pattern "managed child runtimes require explicit development mode" \
  'if cfg\.Dev && \(cfg\.CodexBin != "" \|\| cfg\.ClaudeBin != ""\)' cmd/agentroomd/main.go
require_source_pattern "production refuses configured in-process managed runtimes" \
  'managed runtimes are disabled outside development mode' cmd/agentroomd/main.go
require_source_pattern "public ingress explicitly denies adapter paths" \
  'respond @private "Not Found" 404' deploy/caddy/Caddyfile.example

((failed == 0)) || exit 1
printf 'Source authorization boundary contract passed\n'
