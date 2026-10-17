# Active Context

**Last updated:** 2026-10-17

**State:** QA

**Substate:** RUNNING

**Task status:** GITHUB OBSERVATIONS — LOCALLY VERIFIED, MERGE QA

## Current Task

2026-10-17: Michael approved implementing the read-only GitHub monitor, making
it work locally and merging it, then returning to desktop/spatial interface
work. Branch `feat/github-observations` includes the preceding verified,
unmerged spatial/coordination foundation. Development PR
[#7](https://github.com/msitarzewski/agent-room/pull/7) is in draft while final
checks complete. Backend checkpoint `5094f3d` is published; reviewed namespace
hardening and the final collector/docs are awaiting their consolidated commit.

Implementation and local acceptance pass: 53 collector tests (30 GitHub, 23
Sentry), backend vet/race/SQL/API coverage 84.9% with all critical packages above
90%, frontend 140 tests/30 native browser checks, type/lint/API, static/security,
license/advisory/secret/completeness gates. Independent review's identity,
partial-data, pagination, freshness and CI-attempt findings are fixed and tested.
Initial CI passed backend, real OIDC browser, security and CodeQL; Linux Firefox
revealed the earlier layout test's incorrect graphics-only assumption. That test
now verifies either graphical or accessible layout with the same information
and control requirements; focused Chromium/WebKit checks pass. Final CI rerun
and merge remain required.

Native backend candidate is active with dirty-build provenance, retaining the
verified frontend `5ef2b30`. `local.agentroom-github` now polls this public repo
with outbound anonymous GETs every 900 seconds. Its separate Agent Room token
has only event:ingest/source:github for this one project. Seven actual PR records
were collected (no standalone issues currently exist); PR #7's real CI failure
appeared as a source blocker in authenticated HTTPS inspection. No canonical
completion, ownership, human decision or execution was inferred. All 230 prior
canonical task records retain their checked fields. Sentry's 99-project
publisher was reloaded with shared delivery; both queues have zero pending or
quarantined records and no collection/transport error at the recorded checks.

Runtime/config/credentials and detailed acceptance evidence remain ignored in
`data/local/github/`; consistent pre-install database/native/web/config/Sentry
queue snapshots are in `backups/pre-install` there. Exact candidate revision,
activation phase and per-artifact provenance are in private deployment metadata.
Root owns final CI/docs/Git merge and clean activation; backend/data/review
workers are finished with their owned source frozen. Source services and remote
host remain separate from agent execution or future interface redesign.

Track [GitHub observations](releases/github-observations.md). Next complete
final PR checks, merge, rebuild/activate clean merged provenance, verify merged
PR source state, then resume the app's interface/spatial work. Physical device
comfort/frame pacing and remote promotion remain unverified.

## Previous Information Layout

Michael's necessary-information overlap follow-up is implemented on
`feat/spatial-workspace` and committed as `5ef2b30`, extending baseline
`14b5c43`. Desktop connection
header/details, toolbar, notices, canvas, and hint reserve separate flow space.
Long identities wrap; narrow/200%-text inspectors and confirmation controls stay
reachable. Confirmation receives focus and returns it to its trigger on cancel,
including Safari's click-focus behavior.

Immersive graph/node/label bounds fit a reserved central slot. Coordination,
human escalations, task/request inspector, navigation, diagnostics, and handle
use separate spaces. Texture text fits measured glyph ink; complete long task
titles and request/evidence/source context remain pageable. Existing renderer,
authorization, and versioned actions are reused; no new dependency or service.

Verification passes: 140 frontend tests, 95.24% line / 84.63% branch coverage,
30 native Chromium/WebKit browser checks, type/lint/strict API/build, eleven
security rules, npm advisory and Git-visible secret/completeness checks. Actual
authenticated tailnet HTTPS verifies the existing task within the 99-project
portfolio at 1440px, 760px, and 390px/200% text: separate information, reachable
controls, no overflow/application/API/CSP errors. Private screenshots and
rollback copies are ignored. Physical headset comfort/frame pacing remain
unmeasured; CPU scene geometry and browser checks do not substitute for them.

Verified frontend assets are installed atomically, retaining older hashed assets
for open tabs. The clean `5ef2b30` build matches the accepted candidate byte for
byte; all nine installed files were verified through tailnet HTTPS. Native
binaries remain the verified `14b5c43` build; no daemon,
Sentry publisher, database, identity configuration, or permissions changed.
Frontend revision/hashes and current activation phase are recorded separately in
`data/local/deployment.json` and
`data/local/coordination/logs/ux-layout-build.json`; check those on recovery.
The authorized source checkpoint and final clean frontend activation are
recorded in that private metadata, which is authoritative for deployed artifact
state. See [information layout follow-up](releases/agent-coordination-workspace.md#information-layout-follow-up--2026-10-15).

Final source-publisher check for this release found 99 projects, zero pending
events, zero quarantined events, and no collection or transport error. This is
the recorded release check, not a claim of a fresh scan on every docs update.
Recovery copies are private in `data/local/coordination/backups/ux-layout-before`.
Next: inspect dense information and input on the actual Vision Pro, measure
comfort/frame pacing, deliberately onboard real agent runtimes with scoped
authority, and verify the eventual pipx host before remote promotion. Michael
remains mostly a passenger; agent coordination is the primary product workflow.

## Previous Local Release

The approved iteration is implemented and verified locally. Four semantic
regions replace the anonymous 99-project field, with twelve-project pages,
attributed requests/work, and smaller human escalations across desktop/XR.
Typed REST/MCP requests and metadata-only native-session reporting were
exercised by the actual coding participants: ownership, evidence, peer response,
and resolution. The peer is disconnected/closed; the owned release task is in
review, with completion acceptance separate from source checklist completion.

Final gates pass: 137 frontend tests, 28 native browser checks plus four final
layout checks, backend coverage 84.7% with every critical package above 90%,
and security/license/advisory/completeness checks. Actual authenticated HTTPS
Chromium (WebGL 2) and WebKit (WebGPU) verify all 99 summaries, semantic groups,
paging/return, real request inspection, narrow layout, CSP, and live sockets.
Runtime source-only and audit privacy findings are fixed. Existing 16 other
human task records are unchanged. Sentry remains independently supervised with
99 exact source-only project scopes and no delivery backlog/quarantine.

Implementation commit `796c235` is verified from a clean checkout: native and
Linux daemon/CLI provenance records the exact revision with vcs.modified=false;
the pinned web build matches installed assets byte for byte. Readiness is 204,
HTTPS workspace is 200, and committed-build evidence is attached to the actual
review task. Both temporary participants are disconnected, with root session
idle and peer session closed. Their queues are empty; both acceptance tokens
are revoked and raw credentials removed. Durable identity state is retained.

The final documentation checkpoint uses the same clean rebuild/activation
procedure. Exact deployed revision, hashes, and recovery phase are private in
`data/local/coordination/final-state.json` and `data/local/deployment.json`;
check those on recovery before assuming the last operation finished. No remote
activation, provider-hook service, or autonomous executor is claimed. Next are
physical Vision Pro interaction/comfort/frame pacing, deliberate runtime
onboarding, and verified pipx promotion. Keep private runtime/evidence ignored.

2026-10-15: Michael approved proceeding with the agent-first plan. Implementation
is authorized on the current feature branch: typed machine requests, narrowly
authorized peer coordination, global actionable summaries, grouped semantic
desktop/XR views, explicit human escalation, and honest runtime integration.
Read [Agent coordination workspace](releases/agent-coordination-workspace.md)
for the stable checklist, ownership, authority, and acceptance record. Preserve
the prior Memory Bank edits, 99-project deployment, and ignored private runtime.

2026-10-15 product clarification: Michael says this project is really for
agents and he is mostly a passenger. Treat agents as the primary participants;
center work, ownership, requests, dependencies, reviews, and handoffs. Revise the
human-centered Needs You proposal into a coordination view with a smaller human
escalation surface. APIs/shared records are the agents' workspace; desktop/XR
are the human's window and occasional participation surface. Core project brief,
product context, rules, and decisions now preserve this clarification. Sentry
supplies real repository/task context, while live participation still requires
real runtime observations and request/handoff contracts through existing
agent/run/session/task/adapters. This is product direction, not a blanket grant
of execution authority or a claim that autonomous coordination is implemented.

2026-10-15: Michael tested the full immersive portfolio and reported that 99
similar icons do not communicate useful grouping, urgency, or asks. He asked
for thoughts about meaningful color/grouping. The original question was design feedback; Michael subsequently approved
implementation as recorded above.
Read-only inspection found one open attention item and one source-blocked task,
with no recorded running runs, pending approvals, or pending completion claims.
The current scene gets only the selected project's attention/resources; global
portfolio summaries are used by the HTML directory but are not passed into XR.
Project glyphs share the same default color and identity-based positions; labels
above 16 nodes are hidden unless selected, hovered, or carrying attention.
Propose a shared authorized coordination signal summary, named regions,
progressive cluster expansion, semantic color with labels/badges, and in-world
agent requests/handoffs with a separate human-escalation surface. Preserve stable geography and
do not infer live workers or human approvals from Sentry/Git activity. Extend
existing overview, scene, labels, and tests; no replacement renderer or new
background process. See the updated zoomable control room plan's portfolio
feedback section. Full portfolio setup below remains complete.

2026-10-15: Michael explicitly approved provisioning all discovered Sentry
projects, full project permissions for his operator account, and source-ingestion
access only for the publisher. This resolved the earlier automatic-review scope
rejection. Fresh discovery found 99 projects; 98 were added, and all 99 operator
memberships now have the established full project capability set. The existing
publisher covers those exact IDs with only `event:ingest,source:sentry`; its
previous credential is revoked and removed. No additional process was installed.
Both databases and the queue were backed up privately. All 99 source reports
were delivered with no backlog, quarantine, unmapped project, or delivery error.
Chromium/WebKit authenticated portfolio and project selection checks pass.
The 13 pre-change tasks retain their human-owned fields; a new real source task
was added during collection. Secrets, host values, and detailed evidence remain
ignored under owner-only `data/local/`. No runtime file is tracked. Full portfolio
operation is complete; headset workflow/comfort, brief polish, and pipx promotion
remain the next work.

2026-10-15: Michael reports, "Immersive works!" User-reported immersive operation
is confirmed. Detailed selection/inspection, exact device/software build,
comfort, recovery, and measured frame pacing remain to be recorded in
`releases/spatial-workspace.md#remaining-device-and-promotion-acceptance`.
Next recommendation: expand the observatory's real project portfolio with
view-only operator access, improve the brief's routine-scan wording/counting,
finish the headset workflow checks, then prepare the verified pipx deployment.
These were recommendations at the time of the headset confirmation. The explicit
approval and completed full-permission portfolio setup are recorded above.

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

The independent publisher now collects all 99 approved source projects every
300 seconds with durable retry. Its initial single-project deployment contained
13 real task records. Automatic approval review originally rejected a 97-project
permission expansion; Michael subsequently approved the exact scope and the
fresh 99-project setup is complete. Actual authenticated Chromium/WebKit
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
