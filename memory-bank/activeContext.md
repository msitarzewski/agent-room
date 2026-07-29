# Active Context

**Last updated:** 2026-07-28

**State:** APPLY

**Substate:** RUNNING

**Task status:** APPROVED — APPLYING

## Current Task

The approved production-ingress simplification is being published and deployed.
Caddy owns public HTTPS and proxies to Agent Room over HTTP on
`127.0.0.1:8443`. Application, administration, and adapter listeners are
loopback-only. The application no longer manages an upstream private CA, TLS
certificates, client certificates, or a firewall rule for port 8443.

## Open-Source Release

- Apache-2.0 project license with Michael Sitarzewski as copyright holder
- public contribution and private vulnerability-reporting policies
- full reviewed runtime dependency license texts and required notices
- release archives fail closed when legal material is absent
- CI license policy gate for Go and browser runtime dependencies
- OpenAPI license metadata changed from proprietary to Apache-2.0
- private workstation paths, personal infrastructure names, and personal test
  fixtures removed
- public deployment examples use generic hostnames and loopback addresses; the
  Hermes participant remains Pip / `agent_pip`
- source secret, static analysis, unit, browser, release reproducibility,
  completeness, and deployment gates pass
- public Git history contains no gitleaks findings
- public repository: `https://github.com/msitarzewski/agent-room`
- CI repair: exact Playwright container 18/18 and fresh PostgreSQL 18 + pinned
  Dex real-daemon OIDC 1/1 pass
- repaired Semgrep 1.124.0 environment scanned 73 source targets with zero
  findings

See
`memory-bank/tasks/2026-07/270726_open-source-publication-readiness.md`.

## User Requirements

- update the founding specification with the approved mission and product refinements
- create the Memory Bank from that specification
- plan for a development-to-production mechanism
- target a dedicated Ubuntu Linux/amd64 `host_agentroom` for production
- use co-located Caddy as the only public listener
- represent the Hermes agent Pip as a participant in Agent Room chat
- develop and debug natively on `darwin/arm64`
- deploy the exact promoted artifact to Ubuntu Linux on dedicated Intel hardware
- use the full agent team and continue repair/test loops through functional,
  unit, browser, agent-turn, security, and penetration testing
- scan and repair TODOs, stubs, skipped tests, and incomplete references before
  completion

## Completed Context

- The product foundation and Memory Bank are complete.
- The implementation plan was explicitly approved on 2026-07-27.
- The selected architecture is a Go modular monolith, PostgreSQL 18, and a
  React/TypeScript browser client.
- Development and debugging target native Apple Silicon; the promoted
  production artifact targets Ubuntu Linux `amd64`.
- Production uses a hardened native `systemd` service, PostgreSQL/pgBackRest,
  and co-located Caddy proxying to loopback HTTP.
- Backend, frontend, and security/deployment work are assigned to separate
  agents with non-overlapping ownership.
- The complete candidate now exists in the isolated sandbox.
- The simplified ingress candidate was explicitly approved on 2026-07-28.
- Caddy 2.11.4 validates the generic six-line configuration and the live
  independently managed site block.
- Native Apple Silicon, PostgreSQL 18 integration, real OIDC browser,
  Linux/amd64 emulation, three-engine browser, security scanner, HTTPS
  boundary, active ZAP, adapter turn, deployment, backup/restore, and
  completeness gates pass.
- Backend statement coverage is 81.5% overall; each critical package is at or
  above 90%. Frontend tests pass their enforced statement, branch, function,
  and line thresholds.
- The final completeness scan found no TODOs, stubs, skipped/focused tests,
  unreviewed suppression markers, or production-like fabricated data.
- Disposable release-test processes, credentials, and databases were kept
  outside source and removed after verification.

## QA Evidence

- Native backend, race, PostgreSQL 18, and API suites pass.
- Backend coverage: 81.5% overall; app 92.1%, artifacts 90.8%, auth 90.2%,
  httpapi 92.2%, runner 91.6%.
- Frontend: 47 unit/component tests pass; enforced coverage, typecheck, lint,
  strict OpenAPI, accessibility, and production build gates pass.
- Browser: native Chromium/WebKit 12/12, Linux/amd64 Chromium/Firefox/WebKit
  18/18, and real-daemon OIDC 1/1 pass.
- Linux/amd64 race/integration gate passes; both binaries are statically linked
  x86-64 ELF and the control CLI executes under emulation.
- Real Codex, Claude, and Hermes turns produced sanitized, versioned adapter
  contract fixtures; the live Hermes turn returned the required sentinel.
- Gitleaks, Semgrep, Gosec, Staticcheck, govulncheck, npm audit, OSV,
  ShellCheck, deployment, backup/restore, authorization, and completeness
  gates pass.
- Digest-pinned ZAP 2.17.0 spidered 10 URLs and completed its active scan with
  no Medium or High findings.
- Initial public release: 208 files, 32,573 insertions.

## Completion Record

Approved and applied on 2026-07-27.

See
`memory-bank/tasks/2026-07/270726_founding-agent-room-release.md`.

## Pending Decisions

- OIDC client registration and secret
- encrypted off-host backup destination and retention
- Hermes API credential for live acceptance

Do not invent external deployment values. Continue local implementation and
production-equivalent verification while those values remain unavailable.
