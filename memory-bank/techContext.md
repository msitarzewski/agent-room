# Technical Context

**Last updated:** 2026-07-27

**Implementation status:** Founding release implemented and applied

## Known Environment

- Development workspace: `<repository-root>`
- Development target: native `darwin/arm64`
- Production target: dedicated Ubuntu Linux/amd64 host identified as `host_agentroom`
- Production participant: Hermes Agent named Pip
- Public ingress host: ingress server named `host_ingress`
- Edge proxy: Caddy on `host_ingress`
- Network: `host_ingress` proxies public HTTPS traffic to `host_agentroom` over the private network

Do not record private addresses, credentials, domain names, or secret material in the Memory Bank.

## Confirmed Integration Surfaces

### Hermes Agent

Current Hermes documentation describes:

- lifecycle hooks in CLI and gateway sessions
- MCP client support
- persisted sessions in SQLite
- a messaging gateway
- REST session/chat management
- webhook ingestion
- Docker and non-Docker deployment modes
- explicit gateway authorization and production hardening

Primary references:

- https://hermes-agent.nousresearch.com/docs/
- https://hermes-agent.nousresearch.com/docs/user-guide/features/hooks/
- https://hermes-agent.nousresearch.com/docs/user-guide/features/mcp/
- https://hermes-agent.nousresearch.com/docs/user-guide/features/api-server/
- https://hermes-agent.nousresearch.com/docs/user-guide/sessions/
- https://hermes-agent.nousresearch.com/docs/user-guide/security/
- https://hermes-agent.nousresearch.com/docs/user-guide/docker/

Integration must target documented surfaces rather than patching Hermes internals.

### Caddy

Caddy supports HTTPS termination, reverse proxying, WebSocket upgrades, active upstream health checks, trusted proxy configuration, and forward-auth integration.

Primary references:

- https://caddyserver.com/docs/caddyfile/directives/reverse_proxy
- https://caddyserver.com/docs/caddyfile/directives/forward_auth

The final Caddy configuration requires the actual public hostname, private upstream address, identity provider choice, and port assignments. These are operational secrets or environment facts and are not invented here.

### Open Standards

- OpenTelemetry: preferred compatibility layer for raw traces, metrics, and logs
- MCP: AI-worker access to Agent Room tools, resources, and prompts
- HTTP/WebSocket: human UI and service integration
- ARP: versioned domain contract for commands, semantic events, evidence, attention, and control

## Selected Stack

- architecture: hexagonal Go modular monolith
- backend: Go 1.26.x, standard `net/http`
- live browser transport: `coder/websocket`; commands remain REST requests
- database: PostgreSQL 18 with `pgx/v5` and generated `sqlc` queries
- migrations: embedded, sequential SQL migrations
- frontend: React 19, TypeScript, and Vite
- browser testing: Playwright across Chromium, Firefox, and WebKit
- component testing: Vitest, React Testing Library, and axe checks
- observability: structured `slog` plus OpenTelemetry traces and metrics
- authentication: provider-neutral OIDC relying party with opaque server-side
  sessions
- production artifact: immutable checksummed `linux/amd64` release archive
- production supervision: hardened native `systemd` unit and AppArmor profile
- backups: pgBackRest with WAL archiving and an encrypted off-host repository

See `memory-bank/decisions.md#2026-07-27-go-modular-monolith-on-postgresql`.

## Open Operational Decisions

Do not invent:

- concrete OIDC provider and client registration
- deployment runner and secure access path to `host_agentroom`
- encrypted backup destination and retention
- public domain and certificate arrangement
- exact authenticated-encryption material between `host_ingress` and `host_agentroom`
- production secret recovery custodian and procedure
- log and telemetry retention

## Required Runtime Capabilities

- native `darwin/arm64` development and debugging
- promoted `linux/amd64` production execution
- deterministic migrations
- graceful process shutdown
- health and readiness endpoints
- WebSocket support through Caddy
- structured logs and OpenTelemetry-compatible export
- explicit configuration validation at startup
- secrets supplied outside source control
- independent Hermes and Agent Room service management
- reproducible versioned builds

## Current Repository State

- the approved implementation is applied to
  `<repository-root>`
- the public repository is `https://github.com/msitarzewski/agent-room`
- the default branch is `main`
- backend, frontend, security, deployment, and test harness gates pass
- the repository is licensed under Apache-2.0
- source and release archives include reviewed third-party license material
- no production credentials or host-specific addresses are stored in source
