# 270726_founding-agent-room-release

## Objective

Implement the approved founding Agent Room release, debug it natively on Apple
Silicon, verify the promoted Ubuntu Linux `amd64` target, and provide a
repeatable development-to-production mechanism for the future `host_agentroom`/`host_ingress`
topology.

## Outcome

- ✅ Approved candidate applied to `<repository-root>`
- ✅ Backend race, PostgreSQL 18, API, and Linux/amd64 integration gates pass
- ✅ Backend statement coverage: 81.5% overall; all critical packages at least
  90%
- ✅ Frontend: 47 unit/component tests plus enforced coverage, typecheck, lint,
  accessibility, strict OpenAPI, and production build gates pass
- ✅ Browser: native Chromium/WebKit 12/12, Linux Chromium/Firefox/WebKit
  18/18, and real-daemon OIDC 1/1 pass
- ✅ Real Codex, Claude, and Hermes turn contracts verified
- ✅ Security scanners, authorization boundaries, active ZAP, deployment,
  backup/restore, reproducibility, and completeness gates pass
- ✅ No TODOs, stubs, skipped/focused tests, unreviewed suppressions, secret
  findings, or production-like fabricated data remain

## Files Added or Extended

- `cmd/agentroomd`, `cmd/agentroomctl` — daemon and administration CLI
- `internal/` — domain, application, PostgreSQL, authentication, HTTP/MCP,
  artifacts, adapters, realtime, runtime control, and system integration
- `web/` — React/TypeScript supervisory interface and browser tests
- `api/` — strict OpenAPI and ARP event-envelope contracts
- `db/` — embedded migrations, sqlc queries, and generated access layer
- `deploy/` — immutable packaging, verification, systemd, AppArmor, Caddy,
  firewall, pgBackRest, deploy, rollback, and restore procedures
- `.github/workflows/security.yml` — pinned cross-platform release and security
  gates
- `tests/` — API, security, penetration, completeness, provenance, deployment,
  and real-daemon tests
- `README.md`, `boot.md`, and `memory-bank/` — product, architecture,
  operations, and durable project context

## Patterns Applied

- `memory-bank/systemPatterns.md#Layered-Control-Plane-Architecture`
- `memory-bank/systemPatterns.md#Command-Event-Projection`
- `memory-bank/systemPatterns.md#Authority-and-Provenance`
- `memory-bank/systemPatterns.md#Hermes-and-Pip-Pattern`
- `memory-bank/systemPatterns.md#Deployment-Boundary`
- `memory-bank/testing-patterns.md`

## Integration Points

- Browser clients use OIDC sessions, project capabilities, REST, and
  WebSocket projections.
- Codex, Claude, and Hermes adapters normalize native structured events into
  ARP with source identity and idempotency.
- Pip participates through least-privilege MCP tools, Agent Room chat,
  evidence, task, and attention capabilities.
- Caddy on `host_ingress` terminates public traffic and reaches Agent Room on `host_agentroom`
  through authenticated TLS; administration and adapter listeners remain
  private.
- Production artifacts are immutable, checksummed, SBOM-backed, signed, and
  promoted without rebuilding.

## Architectural Decisions

- Go modular monolith with PostgreSQL 18 and React/TypeScript.
- Native Apple Silicon development; static Linux `amd64` production artifacts.
- Production in-process worker execution remains disabled until an isolated
  worker service exists.
- Hermes and Agent Room retain independent identities, processes, data,
  credentials, upgrades, backups, and rollback boundaries.

## Security and QA Findings Repaired

- Preserved database infrastructure errors instead of misclassifying them as
  invalid authentication.
- Made content-addressed artifact publication atomic and non-overwriting under
  concurrency.
- Corrected MCP schemas that could panic or violate the SDK contract.
- Added exact production CSP upgrade enforcement.
- Corrected the canonical Gitleaks module path and added pinned Staticcheck to
  local and CI gates.
- Upgraded and digest-pinned ZAP 2.17.0.
- Removed generated binaries and TypeScript build metadata from source scope.

## Remaining Operational Inputs

- public hostname and OIDC client registration
- secure deployment access to the Ubuntu Intel host
- `host_ingress` private upstream details and trust material
- encrypted off-host backup destination and retention
- Hermes service credential and hook/MCP configuration
- public repository hosting and protection configuration

No production credentials, private addresses, or invented external values were
added.
