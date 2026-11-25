# Active Context

**Last updated:** 2026-11-24

**State:** DOCS

**Substate:** IDLE

**Task status:** Shared-host deployment is live and healthy (`086d31a`). Completed observability, Sentry heartbeat/coverage, CLI token-expiry, Attention brief wording, and shared-host `backup`/`upgrade`/`check` hardening on `feat/shared-host-migration`. [Task record](tasks/2026-11/241126_observability-and-ops-hardening.md)

The Timeline pager and event-type filter went live at 18:58 CST on Nov 18. Signed-in Safari and the production API for a high-volume observed project verified the release. [Timeline task record](tasks/2026-11/181126_timeline-pagination.md)

## Final snapshot recovery — 2026-11-18

The first attempt stopped at the missing canonical hardener during final provisioning. The host's committed hardener was restored and verified. Both remote installation markers still identified the rehearsal, and the private upload matched the final export. The three local writers were unloaded. Michael approved and applied the retry patch on `feat/shared-host-migration`. All 136 migration and collector tests passed in the deployment checkout.

The retry preserved the restored databases. The remote health checks passed, including fresh GitHub delivery. The runner completed the local Sentry handoff and wrote the active deployment record. The old local app and GitHub launch agents are unloaded. [Recovery record](tasks/2026-11/181126_cutover-recovery.md)

## Interactive Cutover Started — 2026-11-17

2026-11-18 verification: the earlier Terminal window still existed, but no
migration process held the orchestration lock. The initial window inventory
used a bad AppleScript delimiter; its absence result was incorrect. The same
authorized command was restarted in a new dedicated native Terminal window.
Direct property checks now confirm the current `Agent Room migration` window
is visible, unminimized and frontmost on the deployer's active console desktop.
The inactive previous attempt is labelled separately. No terminal contents or
password input were read. The checkpoint remains in fencing with no final
export/activation at the last check; current private window metadata records
the verified handle. Enter sudo only in the foreground Terminal prompt.

Michael explicitly asked the assistant to run the prepared cutover. This
authorizes executing that reviewed script as the deploy user; all shared-host
isolation and private-data rules still apply. The first PTY invocation verified
the signed release and reached the destination's sudo password prompt before
any new local shutdown/export. It was cancelled there, with no activation.

The assistant then opened and started the same command in a dedicated native
Terminal window titled `Agent Room migration`, allowing Michael to enter the
sudo password directly into the terminal rather than chat/tool arguments.
The command is running and the checkpoint remains at its prior `fencing` phase
while the early remote privilege preflight waits. Source ingestion is caught
up and no final export/remote activation has occurred at the last check.

Private window handle: `data/local/migration/operator-terminal.json`. Monitor
checkpoint phases and Terminal busy state without reading credential input.
Once sudo input is supplied the existing script proceeds automatically; later
SSH phases may prompt again. Verify remote health, actual login, GitHub delivery
and Sentry-over-tunnel receipt before claiming the move complete. Never weaken
sudo, store a password or infer authorization from an elapsed wait.

## Destination Cutover Continuation — 2026-11-17

Michael reaffirmed that the remote host is the goal and asked to keep moving.
The new source credential is accepted. Under the existing collector's process
lock, recovery paused only that publisher, made a consistent private SQLite
backup, and set the existing 401-failed records eligible for retry. No event ID,
body, sequence, occurrence time, status or attempt history was changed; a digest
verified those immutable queue contents. 101,973 queued records were preserved;
only 4,950 authentication retry timers changed. The same supervised publisher
was reloaded and all 99 delivery errors cleared. Queue drainage is complete:
pending 0, quarantined 0, and all 99 projects have current successful delivery.

The destination remains staged, not activated. Read-only checks confirm the
isolated runtime UID has no process collision, proposed app/identity/admin/
adapter listeners are absent, Docker/tools/Tailscale work and space is ample.
Remote sudo requires interactive input; it is not available noninteractively.
The host guide's operator execution boundary remains in force. Michael has been
asked whether a normal Terminal is available and given the concrete cutover
command; no password should be sent here.

Cutover now runs a signed, staged-manifest-bound remote preflight before any
local disable/bootout/export or checkpoint phase change. The reviewed helper is
sent inline over the operator's SSH TTY so the old staged helper need not be
replaced just to perform preflight. It verifies staged ownership/identity,
tools, private port boundaries and sudo access. A failure preserves the source
services and durable state. Separate later SSH sessions may prompt again;
neither password storage nor a cross-TTY authorization cache is introduced.

Reuse analysis for November tracking: the existing October monthly README is
historical and cannot be repurposed as the current month's index. A November
README records this continuing task without inventing a release milestone.
Implementation remains on `feat/shared-host-migration`; no remote service or
configuration has been changed by this continuation.

Final verification: 76 migration tests plus 23 Sentry and 30 GitHub tests pass
(129 total), along with bundle validation, Bash syntax, ShellCheck, deployment
static/completeness and whitespace checks. The local bundle now includes the
reviewed early-preflight helper; its private original helper/manifest are kept
for recovery. The already-staged remote bundle and its durable uploaded hash
remain unchanged; preflight verifies that old hash inline before local fencing.
The final export will still capture both current databases and current GitHub
state before replacing staged data. Source-input baseline remains unchanged.

The remaining step is operator execution of the existing `cutover` command from
an interactive Terminal, entering sudo only at its remote prompt. It verifies
authorization before shutdown, then fences/exports/uploads/restores/starts and
checks the destination, establishes the local Sentry tunnel and updates private
deployment provenance. Browser/headset acceptance and explicit `retire` follow.
No user answer or elapsed wait is approval to bypass sudo or the host guide.

## Sentry Credential Rotation — 2026-11-17

Michael explicitly requested a replacement expiring December 31, 2026. The
existing auth service created it with absolute expiry
2026-12-31 23:59:59 America/Chicago (2027-01-01 05:59:59 UTC), preserving the old
actor, exact 99-project scope and only `event:ingest` / `source:sentry`.
The CLI accepts whole-day lifetimes, so a private ignored one-off helper called
the existing `CreateServiceTokenWithMetadata` API for the exact requested date;
no production code or token policy was changed.

Installed by atomic replacement of the existing mode-0600 credential file.
Database metadata verifies the exact expiry and unchanged scope. One genuine
queued observation with its original event ID received HTTP 202; its queue row
was left for the publisher's normal idempotent acknowledgment. The previous
expired token was revoked. No credential value was printed or tracked.

At verification, roughly 101,900 observations remained queued and the publisher
was still waiting in its existing authentication retry backoff (up to 15
minutes). The running publisher reads the token on every delivery attempt, so
no restart or queue reset is needed. Do not claim the backlog is drained until
its counters and delivery timestamps confirm that. GitHub's disabled state and
the staged migration are unchanged. Private operation evidence and metadata
are under ignored `data/local/sentry/`; the credential remains only in its
protected configured file. Next: verify automatic delivery/queue drainage, then
reassess and resume the operator-run migration.

## System Review — 2026-11-17

Michael requested the latest status and a system walkthrough. Read-only checks
confirm the application is still on the workstation, readiness 204 and public
health/document 200. Anonymous API access is 401 and private ingestion is 404.
An isolated authenticated Chromium walkthrough passed login, all 99 authorized
projects, workspace/live connection, tasks, workers, approvals, health and
sign-out; no page errors, server failures or desktop horizontal overflow.

The migration is unfinished: its durable checkpoint remains `fencing`, with no
final export or activation attempt. The destination is provisioned/restored but
only its isolated PostgreSQL container is running; no app/admin/identity
listener or app Supervisor configuration is active. The local app and Sentry
publisher are running. GitHub remains disabled/unloaded from the October
cutover attempt, and no source tunnel is installed.

Sentry still scans all 99 projects successfully, but its source-ingestion token
expired 2026-11-14. All 99 delivery reports have `http_401`; last successful
delivery was November 14. About 100,500 immutable observations are queued
locally, with no quarantine at this check. Fresh collection and an active web
socket do not mean observations reached the application. GitHub's last delivered
observation is October 20; its token is still within its recorded lifetime.

All 120 focused checks pass: 67 migration, 23 Sentry and 30 GitHub tests, plus
deployment/static/completeness and whitespace checks. Public main still matches
local main at `ee4a897`; the migration helpers/docs remain uncommitted on
`feat/shared-host-migration`. The prepared bundle's source-input baseline is
unchanged, but its age and expired Sentry access must be addressed before
cutover. This review changed no service, credential, queue, host or cutover state.

Next: renew the existing exact-scoped Sentry source credential, verify actual
delivery and drain the preserved queue, then recheck/rehearse the migration and
resume operator-run cutover. Keep GitHub's disabled state explicit until its
single publisher is deliberately brought up as part of that recovery. Do not
clear queues/checkpoints or present the old cutover retry command as sufficient
without source recovery. Automated credential-expiry monitoring remains an
operational gap. Private evidence: `data/local/system-review-20261117.json`,
`system-review-browser-20261117.json` and `system-review-20261117.png`.

The October handoff below is historical; use this review for current health.

## Shared-host migration

Michael approved the move off the workstation and supplied the destination's
canonical deployment guide. Read-only SSH confirms Ubuntu Linux/amd64, Docker
access, the existing site hardener, and available proposed ports during
preparation. The current private checkpoint and read-only destination checks now
confirm the site is provisioned and both snapshots restored. Only the isolated
PostgreSQL container is running; the application is not activated and its
Supervisor configuration is not installed. Exact host identity, addresses and
inventory remain in ignored local files. The assistant's checks remain read-only.

The destination is multi-tenant. Use an isolated site tree/runtime account and
Docker PostgreSQL, preserve neighboring sites and shared configuration, and keep
browser access private over Tailscale. The host guide explicitly requires
idempotent scripts that Michael executes. Prepare and verify those scripts and
the actual migration bundle before handing over the execution commands. The
dedicated-host bootstrap must not be run on this server. Preserve both existing
databases, OIDC subject identity, artifacts, source bindings and durable queues.
GitHub moves to the destination; local Sentry continues through a supervised
private SSH tunnel. Do not retire local hosting before remote acceptance.

Reuse analysis: `deploy/deploy.sh` and `deploy/configure-instance.sh` require a
dedicated root/systemd/native-PostgreSQL host and cannot provision a shared site
without changing their established contract. `deploy/compose/compose.yaml` uses
development OIDC fixtures and cannot be promoted as production identity. Extend
the deployment documentation and reuse `deploy/verify-artifact.sh`, immutable
release assets and the existing collectors in a separate shared-host helper.
Tests exercise migration manifests, path/URL validation and fail-closed
cutover ordering. Work is on `feat/shared-host-migration`.

Implementation passed QA: `deploy/prepare-migration.py` prepares
the signed private bundle; `deploy/shared-host.sh` validates/provisions/restores
only the app's isolated site; `deploy/run-migration.py` stages, persistently
fences local writers, exports a fresh final snapshot, activates/checks the target
and separately retires local hosting after explicit browser acceptance.
`tests/deployment/` contains 67 passing migration regressions, including
credential/config drift, immutable release recovery, private route ownership,
uncertain activation, reboot-persistent fencing and tunnel login persistence.
Both actual database dumps restored in an isolated disposable local cluster;
the pinned Linux verifier accepted the actual signed clean `582567d` artifact.
The existing 53 source collector tests, shell/static/completeness and secret
checks pass. No target mutation or local service change occurred.

## Next: Operator Execution

The host guide's explicit execution boundary remains in force: Michael runs the
prepared commands in an interactive terminal. Staging is complete; next run `cutover`
using `deploy/run-migration.py --bundle data/local/migration/bundle`; inspect the
new URL/login/source freshness and actual headset behavior before explicit
`REMOTE_BROWSER_ACCEPTED` retirement. See
[shared-host migration](build-deployment.md#shared-host-migration).

The source Mac app remains ready (204); remote activation has not begun. A shell
invocation without a real terminal failed before loading the bundle because
remote sudo requires interactive input. The CLI now prints that fixed operational
reason instead of hiding it behind the generic error; unexpected exception
details stay private. Both new CLI regressions and all 22 operator tests pass.
The destination is staged, with its application/admin listeners absent. Private verification is in
`data/local/migration/verification.json`; a future successful operator run writes
`orchestrator.json` and replaces `data/local/deployment.json` with a pointer to
that authoritative remote handoff, retaining the prior record byte for byte.
No remote deployment, restart acceptance, device acceptance or scheduled
off-host backup policy is claimed. Keep addresses/secrets out of tracked files.
Existing current Memory Bank references now use a generic destination label;
historical machine-alias references were not rewritten in Git history.

The subsequent interactive `cutover` stopped/disabled the local GitHub publisher
then halted during fencing because the helper checked label removal immediately
after `launchctl bootout`. Read-only checks find GitHub now unloaded, while local
Sentry and app remain loaded and readiness remains 204. No final snapshot or
remote activation was attempted. The helper now waits up to 30 seconds for both
launchd label removal and the captured process's exit, retaining the final
snapshot safety gate. Two regressions cover delayed successful unload and a
timeout that must never export/start/resume. All 67 migration tests pass. Retry
the same interactive `cutover` command; do not reset the checkpoint or restart
the old GitHub publisher. Private current evidence is `fence-status.json` under
the migration directory.

## Previous Task

2026-10-17: Michael's approved read-only GitHub monitor is working and merged.
[PR #7](https://github.com/msitarzewski/agent-room/pull/7) merged as `582567d`.
It includes the previously verified spatial/coordination foundation and source
integration. Clean native Darwin/arm64 and static Linux/amd64 daemon/CLI builds
record the exact merged revision with vcs.modified=false. The pinned clean web
build matches all nine previously verified installed files byte for byte; web
provenance is updated to the merged revision. Local readiness is 204 and all
nine assets were verified over tailnet HTTPS. Exact hashes/provenance/activation
phase are private in `data/local/github/logs/merged-build.json`,
`data/local/github/final-state.json` and `data/local/deployment.json`.

`local.agentroom-github` watches this public repository every 900 seconds using
outbound anonymous GitHub GETs. Its separate Agent Room token has only
event:ingest/source:github for the existing project. Seven real PR records were
observed; there are no standalone issues at the recorded check. Deterministic
tests cover issue close/reopen. Actual PR #7 exercised draft, failed CI/source
blocker, new-head review, then merged state. Its source blocker resolved while
canonical task state stayed inbox. All 230 preexisting canonical task records
retain their checked fields. GitHub/Sentry freshness remains independent.

Final verification passes: 53 collector tests (30 GitHub, 23 Sentry), frontend
140 tests, 36 native Chromium/WebKit checks and 54 Linux Chromium/Firefox/WebKit
CI checks. Backend vet/race/SQL/API coverage is 84.9%, with all critical packages
above 90%. CodeQL, static/security/advisory/license/secret/completeness/boundary,
real OIDC browser, reproducible packaging and deployment checks pass. Authenticated
HTTPS after clean activation shows the actual merged PR, source details and
healthy socket with no application/API errors. Graphics/fallback layout cases
run independently with the normal test deadline; no interface redesign occurred.

Both supervised publishers have zero backlog/quarantine and no collection or
transport error at final checks: GitHub one project, Sentry 99. Sentry's own
board/watcher/history/backups remain separate. Private pre-install snapshots
include native/web/configuration, both databases and a consistent Sentry queue;
post-acceptance consistent source queue copies are also retained. No runtime
secret or host value is tracked. Source credentials cannot create coordination
commands, accept completion, change ownership or execute workers.

The final Memory Bank handoff is documentation only; it does not require a
runtime rebuild. Deployed artifact provenance remains the clean merged `582567d`
revision even if a later documentation commit advances main. See
[GitHub observations](releases/github-observations.md).

## Next: Return to the Interface

Michael explicitly wants to return to this app's interface, including spatial,
after the monitor. Keep agents primary and the human mostly a passenger. Use
real source review/CI/issue context, ownership and requests to make necessary
information understandable without overlapping panels or anonymous icon fields.
Detailed Vision Pro interaction/comfort/frame pacing still needs device
acceptance. Deliberate runtime onboarding and verified remote promotion remain
separate; no automatic executor or remote activation is claimed.

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
authority, and verify the eventual destination host before remote promotion. Michael
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
onboarding, and verified remote promotion. Keep private runtime/evidence ignored.

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
operation is complete; headset workflow/comfort, brief polish, and remote promotion
remain the next work.

2026-10-15: Michael reports, "Immersive works!" User-reported immersive operation
is confirmed. Detailed selection/inspection, exact device/software build,
comfort, recovery, and measured frame pacing remain to be recorded in
`releases/spatial-workspace.md#remaining-device-and-promotion-acceptance`.
Next recommendation: expand the observatory's real project portfolio with
view-only operator access, improve the brief's routine-scan wording/counting,
finish the headset workflow checks, then prepare the verified remote deployment.
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
release checklist is complete. Mac staging continues; the remote host remains the eventual
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
