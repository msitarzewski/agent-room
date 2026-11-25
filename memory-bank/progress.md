# Progress

**Last updated:** 2026-11-24

## Current Phase

2026-11-24 operational and observability hardening: fixed Sentry `partial` vs.
`complete` classification for readable window-aged task archives, added
`project_heartbeat_seconds` throttling and offline pending-heartbeat coalescing
to reduce `project.observed` event amplification, added service-token expiry
visibility to `agentroomctl doctor` and `service-token expiring`, added
`backup` and `upgrade` modes to `deploy/shared-host.sh`, fixed
`deploy/run-migration.py check` phase preservation, and refined Attention brief
routine-monitoring wording. All Go race tests, Python collector/deployment
tests, frontend Vitest/ESLint/build, and static/security gates pass. See
[observability and ops hardening](tasks/2026-11/241126_observability-and-ops-hardening.md).

Shared-host cutover completed on November 18 at 18:22 CST. The destination passed the remote checks, and all 99 local Sentry projects delivered through the supervised tunnel. Device acceptance and local retirement remain pending. [Cutover outcome](tasks/2026-11/181126_cutover-recovery.md#cutover-outcome)

2026-11-17 migration continuation: local ingestion recovered completely,
pending/quarantined queues are zero and all 99 projects have fresh successful
delivery. Recovery preserved 101,973 immutable observations, backed up SQLite,
and rescheduled only prior 401 retry timers. Cutover now verifies staged
signature/hash, destination identity and operator sudo access before shutting
down any local service. All 129 migration/collector tests and shell/static/
completeness checks pass. The destination remains staged and inactive; the
remaining step needs Michael's interactive Terminal/sudo, followed by actual
remote/browser verification and explicit local retirement.

2026-11-17 credential recovery: Michael approved a Sentry publisher replacement
through December 31, 2026. The exact end-of-day Central expiry, original actor,
99-project scope and ingestion-only capabilities are verified. The credential
was atomically installed, a genuine queued observation accepted with 202, and
the expired predecessor revoked. Publisher retry backoff is still in effect;
the queued backlog is preserved and automatic drainage is not yet claimed.
No service restart or remote activation occurred.

2026-11-17: System review confirms the workstation app/login/live connection and
99-project desktop walkthrough work, with no browser/server failures. All 120
migration/collector tests and deployment/static/completeness checks pass. The
destination is still only staged and the checkpoint remains in local fencing;
remote activation never happened. GitHub remains disabled from that attempt.
Sentry's source credential expired November 14, so collection continues but all
99 projects' deliveries fail 401 and roughly 100,500 observations are preserved
in its queue. Credential recovery, verified queue drainage and supervised
cutover are the immediate operational work. No runtime or credential changes
were made during this review. Migration code/docs are still uncommitted on the
feature branch; public main remains `ee4a897`.

2026-10-20: The approved shared-host migration kit is prepared and verified on
`feat/shared-host-migration`; destination execution remains Michael-run per its
canonical guide. Read-only SSH/platform checks pass. The signed clean `582567d`
Linux/web release, private consistent snapshots, isolated two-database restore
rehearsal and pinned Linux signature verification pass. 63 migration tests and
53 collector tests, shell/static/completeness/secret checks pass. Local app and
publishers still run. Subsequent checkpoint/read-only verification confirms
destination provisioning/restoration is staged, PostgreSQL alone running and
application activation/cutover pending. Noninteractive shell invocation now
reports the terminal/sudo requirement clearly; two new regressions pass. Private
host/credential data remains ignored. See
[shared-host migration](build-deployment.md#shared-host-migration) for actual
stage/cutover/check/explicit-retirement commands and recovery boundaries.

2026-10-17: Read-only GitHub observation support is merged in PR #7 (`582567d`)
and running locally with clean merged native/Linux/web provenance. Seven real
PRs and current-head CI/review/merged states feed the existing workspace without
changing canonical ownership or human decisions. GitHub and Sentry freshness
remain independent; both supervised publishers are healthy at final checks.
53 collector tests, frontend 140/36, backend 84.9% and all CI gates (including
54 Linux browser checks and reproducible packaging) pass. Michael's next focus
is the agent-first desktop/spatial interface. See
[GitHub observations](releases/github-observations.md).

2026-10-15 UX follow-up: necessary information now occupies separate desktop
and immersive spaces, with long recorded context fully available through
wrapping/paging. 140 frontend tests and 30 Chromium/WebKit browser checks pass;
actual authenticated 99-project HTTPS checks cover wide/narrow/200%-text layout.
The clean `5ef2b30` frontend is installed with rollback copies and verified
through HTTPS; native binaries remain `14b5c43` and source services are unchanged.
See [information layout follow-up](releases/agent-coordination-workspace.md#information-layout-follow-up--2026-10-15).

2026-10-15 product clarification: agents are the primary participants; Michael
is mostly a passenger. The approved coordination iteration now implements typed
requests, ownership, reviews, handoffs, and explicit runtime self-reporting, with
human escalations separate. Actual coding agents exercised native-session
reporting, MCP ownership/evidence, and peer request/response/resolution. The
99-project desktop/XR portfolio uses four semantic regions and twelve-project
pages. Full isolated gates and actual Chromium/WebKit HTTPS acceptance pass.
Automatic provider hooks and worker execution are still absent. See
`decisions.md#2026-10-15-agents-are-the-primary-participants`.
Read [Agent coordination workspace](releases/agent-coordination-workspace.md)
for the complete verification and remaining device/promotion checks.

First spatial workspace release candidate is complete locally on
`feat/spatial-workspace`. The observatory, authorized snapshots, typed Sentry
observations, accessible inspection, safe controls, and WebXR implementation
are built and locally verified. The Mac staging deployment is running over
Tailscale HTTPS with OIDC, persistent databases, supervised service operation,
and verified live updates in Chromium/WebKit. Physical Vision Pro acceptance
and remote production promotion remain separate.
[Release progress](releases/spatial-workspace.md) is tracked by Sentry.

Spatial checkpoint `95f166b` and continuous-feed implementation `880b5e1` are
committed. Michael explicitly approved full portfolio setup on 2026-10-15:
99 projects are now provisioned with full operator project permissions and an
exact-scoped, source-ingestion-only publisher. Durable delivery and live
Chromium/WebKit portfolio checks pass, with no backlog or quarantine. Source
collection, delivery receipt, and connection health have separate labels. The
earlier automatic-review scope rejection is resolved by that approval. Read the
[continuous-feed release](releases/continuous-sentry-observations.md).

## Completed Foundations

- [x] Refined mission from passive awareness to trustworthy supervision
- [x] Defined initial customer and coding-agent wedge
- [x] Established attention and evidence as core product concepts
- [x] Clarified command, event, projection, and telemetry boundaries
- [x] Separated agent, instance, run, session, and host identity
- [x] Defined Pip as a persistent Hermes participant
- [x] Defined co-located Caddy and loopback Agent Room production topology
- [x] Defined build-once development-to-production contract
- [x] Reordered roadmap around validated supervisory outcomes

## Implementation Roadmap

### Milestone 1: Real Work Ingestion

**Status:** Implemented and verified locally

- minimal local daemon
- event store and deterministic projections
- one automatic coding-agent adapter
- source provenance
- reconnect-safe ingestion

### Milestone 2: Timeline and Evidence

**Status:** Implemented and verified locally

- browser UI
- active work and timeline
- task state
- evidence and artifacts
- replay and filtering

### Milestone 3: Attention Inbox

**Status:** Implemented and verified locally

- situation detection
- attention lifecycle
- deduplication and acknowledgement
- brief and recommendations

### Milestone 4: Safe Human Control

**Status:** Implemented and verified locally

- approvals and interventions
- policy evaluation
- budgets
- command audit

### Milestone 5: Second Runtime Adapter

**Status:** Implemented and verified locally

### Milestone 6: Hermes Adapter and Pip

**Status:** Integration implemented; live Pip configuration pending

### Milestone 7: Production on host_agentroom

**Status:** Simplified promotion mechanism verified; live deployment applying

### Milestone 8: First Orchestrator

**Status:** Not started

### Milestone 9: Voice and Ambient Awareness

**Status:** Not started

### Milestone 10: Vision Pro

**Status:** Local deployment verified; Michael reports immersive mode works (2026-10-15); remaining device acceptance pending

The first observatory reuses the existing control plane and MiradorXR's technical
patterns. It adds desktop WebGPU/WebGL fallback and dedicated WebGL WebXR, with
shared selection, natural-input contracts, and in-world task/evidence inspection.
No headset frame-rate or comfort measurement has been claimed.

## Immediate Next Work

1. Return to the agent-first desktop/spatial interface with real GitHub/source
   context. Accept dense information/input on the actual Vision Pro and measure
   comfort/frame pacing before choosing an identifier for signed promotion.
2. Improve the brief's routine-scan counting and wording at
   `web/src/pages/AttentionPage.tsx:127`; distinguish source checks from meaningful
   work updates. Full portfolio provisioning is now complete.
3. Verify the operator-designated remote machine's platform and deployment
   configuration before signed promotion; the current Mac deployment is staging
   for headset testing. The actual host identifier stays in the private runtime
   record. The July status does not prove current remote activation.
4. Configure Pip's least-privilege Hermes hooks, Agent Room service token, and
   MCP tools.
5. Provision and test the encrypted off-host backup destination.

## Current Blockers

- repository protection and private vulnerability-reporting configuration
- remote identity configuration verification; local OIDC is already operational
- encrypted backup destination and retention
- Hermes production credential and live hook/MCP configuration
