# July 2026 Tasks

## In Progress

### 2026-07-28: Production Ingress Simplification

- [APPROVED/APPLYING] Simplified production ingress to Caddy public HTTPS
  proxying to Agent Room over HTTP on `127.0.0.1:8443`.
- Removed redundant loopback mTLS, certificate lifecycle, and firewall
  artifacts while preserving loopback enforcement, private-route denial,
  encrypted credentials, OIDC, systemd hardening, and PostgreSQL 18.
- Full backend, frontend, browser, OIDC, security, deployment, Caddy, and
  Linux/amd64 gates pass.

### 2026-07-28: Initial Hosted CI Repair

- [APPROVED/APPLYING] Repaired the initial public workflow's Playwright
  container, Dex subject fixture, and Semgrep isolated environment.
- Targeted Linux container, real-daemon OIDC, regression, security, license,
  completeness, and secret-scanning gates pass.
- Failed run:
  `https://github.com/msitarzewski/agent-room/actions/runs/30319218764`

## Completed

### 2026-07-27: Initial Public GitHub Publication

- Published the Apache-2.0 release at
  `https://github.com/msitarzewski/agent-room`.
- Set the canonical Go module and repository references to
  `github.com/msitarzewski/agent-room`.
- History-aware and working-tree secret scans found no leaks.

### 2026-07-27: Open-Source Publication Readiness

- Licensed Agent Room under Apache-2.0 with Michael Sitarzewski as copyright
  holder.
- Added governance, full runtime attribution, release/CI license enforcement,
  and public-safe deployment examples.
- Secret, license, static analysis, unit, browser, release reproducibility, and
  completeness gates pass.
- See
  [270726_open-source-publication-readiness.md](./270726_open-source-publication-readiness.md).

### 2026-07-27: Founding Agent Room Release

- Implemented and applied the approved control-plane release.
- Native Apple Silicon, PostgreSQL 18, real OIDC, three-engine browser,
  Linux/amd64, adapter-turn, security, active penetration, deployment,
  backup/restore, and completeness gates pass.
- Production promotion is ready; live `host_agentroom`/`host_ingress` configuration still
  requires the real environment values recorded in the task.
- See
  [270726_founding-agent-room-release.md](./270726_founding-agent-room-release.md).

### 2026-07-27: Product Foundation and Memory Bank

- Refined Agent Room into a focused human control plane for AI workers.
- Established the foundational Memory Bank.
- Defined Hermes participant Pip and the `host_agentroom`/`host_ingress` production topology.
- Defined a build-once development-to-production promotion contract.
- QA passed and application was explicitly approved.
- See [270726_product-foundation.md](./270726_product-foundation.md).
