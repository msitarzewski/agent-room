# Active Context

**Last updated:** 2026-10-14

**State:** DOCS

**Substate:** IDLE

**Task status:** CONTINUOUS SENTRY OBSERVATIONS — LOCAL RELEASE COMPLETE

## Current Task

2026-10-14 follow-up: Michael authorized Memory Bank updates, a commit, and the
next build. Spatial checkpoint `95f166b` is committed. Continuous Sentry delivery
and explicit activity/connection/source clocks are now implemented, tested, and
running locally for the existing Agent Room project. The final implementation
commit includes the release docs; clean native, Linux/amd64, and pinned web
builds are recorded privately with their exact source revision. Physical Vision
Pro acceptance, full portfolio setup, and remote activation remain separate.

## Next Implementation: Continuous Observations

The reviewed spatial release is committed as `95f166b`. Native daemon/CLI,
Linux/amd64 daemon/CLI, and pinned web builds pass from that clean revision;
Go provenance records the exact commit with `vcs.modified=false`. The local
launch agent was restarted on those binaries. Current implementation extends
that checkpoint on the same feature branch. Read
[Continuous Sentry observations](releases/continuous-sentry-observations.md)
for the stable checklist and source/delivery contracts.

1. Reuse Sentry's read-only project/task collection without contacting Vikunja.
2. Persist observations and delivery progress locally before attempting delivery.
   Retry interrupted delivery with stable identity and idempotent events.
3. Publish authorized project/task observations through the private adapter;
   preserve human-owned fields and report source freshness honestly.
4. Keep Sentry's current board, watcher, Git history, and backups independent.
   Connector state and credentials belong to Agent Room's private runtime storage.
5. Distinguish last project activity from current connection health in the UI;
   keep displayed elapsed time current while a connection is idle.
6. Verify initial collection, incremental updates, duplicate/restart recovery,
   unavailable delivery, source isolation, and real authenticated live refresh.

The independent publisher now collects the existing Agent Room project every
300 seconds, with durable retry and no membership changes. Initial delivery
contains 13 real task records with no backlog or quarantine. Automatic approval
review rejected adding all 97 source projects and broad operator access; that
action made no changes. The other 96 discoveries remain explicit provisioning
needs pending Michael's scope decision. Actual authenticated Chromium/WebKit
acceptance passes: one healthy socket, heartbeat, typed project frames, advancing
collection times, unchanged human fields, and no reload or CSP violations. The
release checklist is complete. Mac staging continues; pipx remains the eventual
deployment home.

## Spatial Release Context

Michael asked on 2026-10-13 to move the Sentry/Agent Room exploration into this
repository and preserve the plans for recovery. Proposed direction: Agent Room
owns a zoomable visual workspace; Sentry supplies repository and task
observations; Vikunja becomes an optional planning connector.

Semantic zoom: portfolio → project → task → evidence. Explore WebGPU rendering,
meaningful animation, real dependency graphs, source freshness, recorded-event
playback, and an attention queue available at every scale. Reuse the existing
Go/PostgreSQL control plane, React inspectors, and reconnecting event stream.

**Scope authorized now:** Michael requested planning and building the first
release, maintaining Sentry release docs through iteration, and repairing
unfinished code. Work is on feat/spatial-workspace; the existing security edits
are preserved. Michael subsequently authorized local deployment using MiradorXR's
Tailscale Serve pattern, with a dedicated port. Public production promotion and
headset acceptance remain separate.

Michael clarified that the operator-designated remote machine is the eventual
home. This Mac deployment is the local staging/headset-testing step. The actual
remote host identifier is preserved in private `data/local/deployment.json`;
verify its platform before choosing the promoted artifact. Remote activation
has not been requested in this step.

## Local Tailnet Deployment — 2026-10-14

Reuse MiradorXR's loopback + Tailscale Serve HTTPS pattern at
`../miradorxr/vite.config.js:123`. Agent Room gets port 4445; the existing
MiradorXR and other tailnet routes are preserved. Reuse the native Go daemon,
production web distribution, PostgreSQL 18 and Dex compose services, existing
OIDC/bootstrap/migration commands, and private admin/adapter listeners. Actual
hostnames, credentials, configuration and deployment evidence belong in ignored
`data/local/`, not tracked Memory Bank files. Authenticated browser, secure-context,
WebSocket and restart checks pass.

The authorized native service now runs under the per-user `local.agentroom`
launch agent. Dedicated PostgreSQL/Dex containers use persistent databases,
unique private credentials, and restart policies. HTTPS app port 4445 and OIDC
port 4446 are configured in Tailscale Serve; prior mappings are preserved.
Five actual repository task records were imported once through the private
adapter. Live updates work. Database, identity provider, and daemon restart
passed readiness and preserved the existing session and five actual task records.
A real sign-in check exposed a lost workspace return path; the existing App and
AuthContext now preserve and validate it. Exact pinned frontend gates pass with
115 tests and the rebuilt web distribution is serving locally. Login credentials
are in private `data/local/secrets/login.json`; do not copy them into tracked docs.

Sustained verification exposed and repaired a real WebKit compressed-frame
failure and disconnected sockets retaining limiter slots. Isolated A/B proved
both compression modes fail; the control stream now runs uncompressed with one
reader for control/close frames. Both live HTTPS engines receive all five replay
events and heartbeat over one open socket. Deep-link login, secure cookies,
logout, private boundaries, and 24 native browser regressions pass. WebKit WebGPU
and Chromium WebGL visibly render. Application CSP is unchanged; Playwright's
WebKit screenshot helper injects a blocked `body {}` stylesheet, so the strict
application CSP check runs without screenshot capture.

The complete backend race/integration/coverage gate passes: 82.7% overall and
every critical package above 90% (HTTP 92.2%). Ping/close, unexpected client data,
declined compression, and eight reconnect cleanup regressions pass. Vet,
Staticcheck, Semgrep, and Gosec pass. Native binaries were rebuilt and the local
launch agent restarted successfully; final Linux/amd64 static builds also pass.
Source secret scanning inventories Git-visible current source; fixtures prove
source/history leak detection while ignored private runtime files stay excluded.
All temporary test infrastructure is removed; the actual local service stays up.

Final actual-source refresh reached the UI correctly (`source_status=Done` while
supervised status stays inbox), but revealed an inspection gap: human-owned
description correctly retains the first imported 8/10 checklist, while latest
10/10 text is in `source_observation.description` and was not displayed. Existing
desktop/XR inspectors now expose a separately labeled latest source report,
including full description, title, status, time, and reference. XR text paginates
fully; canonical human fields stay intact. Exact pinned gates pass 115 frontend
tests and 24 browser checks. Real HTTPS WebKit confirms the latest 10/10 report
and preserved human fields. The final five-record source update reached the UI
without reload; Sentry's read-only parser recognizes the release as 10/10 Done.
The one-time publisher was revoked, rejected with 401, and its secret removed.
The operator login remains available in the private login file. No further local
product/deployment work remains; continue with actual headset acceptance.

Read [First spatial release](releases/spatial-workspace.md) for current scope,
checklist, iteration evidence, and recovery position. MiradorXR supplies the
technical reference; Agent Room's visuals must be entirely new and magical.

Implementation and local validation are complete. Real Sentry records
have been ingested into a disposable pilot through the private adapter endpoint;
source status and human supervision stay separate. The pilot found and repaired
an int64 sequence SQL bug. Projection rebuild now preserves trusted command
state and source ordering. Desktop WebGPU/WebGL fallback, separate WebGL XR,
accessible inspectors, attention, and safe controls are implemented. Resume
human/device acceptance from the release record; do not claim headset acceptance.
Frontend 115 tests and 24 Chromium/WebKit acceptance checks pass, including
mobile overflow repair. License and Linux/amd64 build checks pass. Final diff
review repaired a preexisting collection cursor skip with 123 tied-timestamp
task traversal, XR run-linked evidence, and dependency-cycle disclosure.
The full security/coverage gate passes on Go 1.26.9 and x/text 0.41.0; the fresh
advisory database required these patch updates beyond the previous repair.
Backend coverage is 82.7%; each critical package exceeds 90%. Exact pinned
Node 24.8.0/npm 11.6.0 source gates pass. The Linux/amd64 CLI executes under
emulation. Linux Firefox 12/12 and final-layout/headed software-WebGL rechecks
pass. Wrapped-layer label overlap is repaired. The original eight-item release
candidate checklist is complete, followed by two verified local-deployment items.
Earlier disposable pilot processes/databases and credentials are cleaned up.
That candidate milestone created no commit, PR,
signed archive, or deployment; the subsequent local deployment is recorded above.
Automatic production Sentry publishing remains a separate connector.

Read [Zoomable control room plan](tasks/2026-10/zoomable-control-room-plan.md)
and [October tasks](tasks/2026-10/README.md) before continuing. Resume the approved
first-release scope. The task includes source
references, Sentry review findings, integration gaps, acceptance criteria,
and the next-session prompt.

## Existing Security Task

**State:** Commit authorized with the spatial checkpoint. Preserve the existing
changes originally on fix/weekly-security-scan and now incorporated in the
feature checkout; Michael's 2026-10-14 commit request covers this reviewed repair.

The scheduled Monday `Security` workflow has failed since 2026-08-10 on
unchanged code. Branch `fix/weekly-security-scan` repairs all causes:

- Go toolchain `go1.26.5` → `go1.26.8` (six reachable standard-library
  advisories, GO-2026-6218/6090/6089/6088/5972/5026)
- web dev dependencies: vitest 4.1.11, brace-expansion 5.0.12, nanoid 3.3.20
  (`npm audit`: 0)
- `internal/postgres/repository_integration_test.go` event timestamp is now
  relative to the clock; the fixed 2026-07-27 value aged out of the 30-day
  ingest window
- removed the OSV `GO-2026-5932` ignore that expired 2026-10-27; osv-scanner
  Go call analysis reports unreachable `x/crypto` advisories as uncalled and
  the `openpgp` dependency guard remains

Local QA passed: full `static-security.sh` against pinned PostgreSQL 18, the
frontend gates, deployment/completeness/license gates, and Linux-container
artifact fail-closed and reproducible-release gates.

The spatial release's fresh 2026-10-14 scan additionally found 11 reachable
standard-library advisories fixed in Go 1.26.9 and GO-2026-6629 fixed in
`golang.org/x/text` 0.41.0. The current feature checkout extends the repair with
those patches; the consolidated security gate passes without new suppressions.

## Previous Task

PR #2–#6 merged the production-ingress simplification and release fixes on
2026-07-29/30. Live production activation is not recorded.

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

## Founding Release QA Evidence (July 2026; historical)

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
