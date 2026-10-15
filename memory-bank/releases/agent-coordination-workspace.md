# Agent coordination workspace

**Updated:** 2026-10-15
**State:** COMPLETE LOCALLY — checklist 9/9; device/promotion checks separate
**Authorization:** Michael approved the agent-first coordination and portfolio-legibility plan after clarifying that agents are the primary participants and he is mostly a passenger.
**Branch:** `feat/spatial-workspace`
**Baseline:** `880b5e1`, with the already authorized Memory Bank and 99-project operational updates preserved.

## Objective

Make the shared machine interface and desktop/XR workspace useful for agents
coordinating real work: ownership, requests, dependency waits, reviews, handoffs,
evidence, and progress within explicit authority. The human can observe and
occasionally participate; human escalations are a separate, smaller surface.
Replace the anonymous 99-project presentation with meaningful stable regions,
semantic color and text, progressive quiet-context expansion, and real global
coordination signals.

## Release checklist

- [x] Implement typed agent coordination requests with ownership, recipients, and task/evidence references.
- [x] Enforce project isolation, recipient authority, versions, idempotency, attribution, and source-token restrictions.
- [x] Expose authorized actionable portfolio summaries with bounded items and honest unknown/freshness states.
- [x] Build stable named work/coordination/context regions with semantic labels and progressive expansion.
- [x] Share coordination inspection and a smaller human-escalation surface across desktop and immersive views.
- [x] Integrate metadata-only real runtime observations through existing adapter foundations without fabricated activity.
- [x] Verify request workflows, signal propagation, grouped 99-project views, accessibility, security, and builds.
- [x] Validate the local deployment and real authenticated portfolio while preserving existing Sentry operation and secrets.
- [x] Review unfinished references and update release, architecture, operating, and recovery documentation.

Keep this path and checkbox wording stable for Sentry task identity. No release
number is assigned. Physical headset comfort/frame pacing, remote pipx activation,
and autonomous worker execution remain outside this local acceptance claim.

## Reuse analysis

`memory-bank/releases/spatial-workspace.md` and
`memory-bank/releases/continuous-sentry-observations.md` have completed stable
checklists for previous capabilities. Extending either with a different workflow
would conflate release progress and source identity. The existing zoomable plan
preserves design history; this separate release file records implementation and
verification of the newly approved workflow using the same Sentry release pattern.

Extend `internal/domain/models.go`, existing command/service methods, resource
storage/projection, HTTP/MCP handlers, and API contracts for typed requests.
Extend `internal/postgres/repository.go` overview summaries and the bounded loader
at `web/src/workspace/data.ts`; avoid full-resource fanout across all projects.
Extend `web/src/workspace/scene.ts`, shared SceneCanvas/Label/TaskInspector, and
WorkspacePage for presentation. Existing Codex/Claude/Hermes normalizers are the
runtime integration foundation. No replacement backend/renderer or new scheduler
is part of the plan. Justify further new files before creation.

## Authority and evidence

- Agents coordinate through machine-readable records and narrowly authorized tools.
- Source observations do not create trusted commands, approvals, or human task state.
- Explicit requests have an authenticated owner and a project-scoped recipient.
- A request, review, or handoff records provenance and evidence; resolving a
  request does not implicitly accept a task completion or run unsafe actions.
- Source repository activity is distinct from running agent work. Missing,
  partial, denied, stale, or disconnected data never becomes a quiet/healthy claim.
- Agent presence and runtime events must describe actual observed work. Tests
  use isolated fixtures; deployed records must come from real authorized activity.
- Sentry's source publisher retains only `event:ingest,source:sentry` for its
  exact approved projects. New machine coordination identities require narrow,
  project-scoped permissions independent of that source credential.
- Secrets, host values, raw source paths, and detailed deployment evidence remain
  owner-only and ignored under `data/local/`. Honor `.gitignore` throughout.

## Verification

Local capability acceptance is complete: 137 frontend tests, 28 native browser
checks plus four final fit checks, backend 84.7% statement coverage with all
critical packages above 90%, and security/license/completeness/advisory gates.
Actual authenticated Chromium/WebKit each load 99 authorized summaries, show
the four real regions, unfold disjoint twelve-project pages, retain return
position, inspect the actual peer response, and preserve narrow-screen layout,
CSP, and a healthy current stream. Chromium uses WebGL 2; WebKit uses WebGPU.
These browser results do not measure physical-headset comfort or performance.

Two actual coding participants supplied their own native session metadata.
MCP recorded this agent's task ownership, working state, and test evidence.
The named peer responded; the owner resolved the request at version 3, then
requested review of the release task. That pending task acceptance is distinct
from this implementation checklist and does not imply a running provider job.
The peer's final disconnected/closed observation was delivered. Temporary
acceptance credentials are closed/revoked during final operational cleanup.

All 99 source project mappings retain source-only access; the publisher has no
backlog, quarantine, unmapped project, or delivery error. Of 99 scan reports,
52 are complete and 47 partial; known work/asks remain visible with coverage
qualifiers. Other preexisting human task fields are preserved. Source-only
request reads, outside-project runtime reads, and public bearer use return 403.
Runtime mutation denial and historical audit privacy are proven in isolated
SQL/API regressions. Secrets and detailed evidence remain ignored and private.

Implementation checkpoint `796c235` is committed and clean-built for native
Apple Silicon and static Linux/amd64 daemon/CLI, with exact embedded revision
and vcs.modified=false. The pinned web rebuild matches installed assets byte
for byte; clean activation returns readiness 204 and HTTPS workspace 200.
Readable committed-build evidence is attached to the actual release task.
Root's final reporting state is disconnected/session idle; peer is disconnected/
session closed. Both queues are empty, both temporary tokens revoked, and raw
credentials removed. Preserve durable reporter identity state for future use.

This final documentation checkpoint is rebuilt through the same procedure.
Exact final source provenance, asset hashes, rollback snapshots, and operational
phase are recorded under private `data/local/coordination/` and
`data/local/deployment.json`; verify them after interrupted execution.

Use isolated database/API/MCP tests for authorization, recipient isolation,
concurrent version changes, idempotent retries, terminal states, source rejection,
summary precision, and replay. Use pinned frontend gates and Chromium/WebKit
acceptance for real region labels, collapsing/expansion, cross-project signals,
unknown data, natural input continuity, and narrow-screen accessible controls.
Verify actual local HTTPS access and the 99-project data set after installation;
measure no physical-headset performance indirectly from browser tests.

## Iteration log

- Michael approved implementation on 2026-10-15. Root owns integration, security,
  local operation, and docs; backend owns typed workflow/contracts; frontend owns
  shared desktop/XR presentation; runtime audit reuses adapter foundations.
- Prior local aggregate: one open attention item and one source-blocked task,
  no running runs or pending approvals/completion claims. These are observed
  baseline counts, not production fixtures.
- Backend/frontend agreed an additive authorized `overview.coordination`
  summary, bounded attributed items, and `workspace.resources.requests`.
  Named regions and progressive pages use actual signals; source freshness and
  partial/unknown coverage remain separate from operational urgency.
- Runtime audit found codecs are not wired as live reporters and generic
  projections omit agent/run identity links. Some provider normalizers could
  forward raw tool/usage/error data. The implementation now includes narrow
  typed runtime observations, privacy allowlists, and a metadata-only explicit
  CLI reporter with durable retry and five-minute presence leases. This uses
  actual native session IDs and never reads conversations or starts workers.
- Reporter separation analysis: existing `internal/adapters/normalize.go` owns
  pure mapping, and the Python Sentry collector is a separate source/runtime.
  Go reporter/transport tests need a focused new file for private HTTP delivery,
  durable retry, and filesystem security without introducing Python or a new
  library dependency into the CLI. The CLI entry point is reused before its
  database initialization. CI's existing backend gate now also runs CLI tests.
- Shared contract: typed requests have an authenticated owner and named agent
  or human recipient, bounded body, same-project task/evidence links, and a
  versioned response/outcome lifecycle. Privacy applies to resource reads,
  snapshot summaries, recorded event replay, and live streams. Source ingestion
  cannot create coordination commands. Existing full operator capabilities can
  observe/manage; machine identities receive narrow capabilities.
- Four named portfolio regions: Coordination, Work recorded, Quiet, Coverage
  unknown. Quiet requires complete authorized coverage and a complete source
  collection within 15 minutes; partial/failed/invalid data stays qualified.
  Known actual requests/work take priority over incomplete coverage. Pages show
  12 projects, with stable region anchors and return/selection continuity.
- Root saved private native, web, and configuration rollback copies before
  adopting changes. Interim frontend builds use a temporary output directory so
  the running instance continues serving its verified assets until installation.
- Planned actual runtime acceptance uses the currently working coding agents'
  own metadata and native session identifiers, with exact project-scoped
  credentials and an actual peer request/response. No provider turn ID, worker
  conversation, synthetic production actor, or autonomous background executor
  will be fabricated. Presence reports expire after five minutes and are closed
  or made idle when the observed work ends.
- Initial integration review: deployment-static, source authorization boundary,
  completeness, redistributed license verification, 23 isolated Sentry collector
  tests, and Git-visible secret scanning pass. Backend SQL/API/MCP and frontend
  scene validation are still running; these preliminary checks do not complete
  the release gates. Review also requires a nonblank human-escalation reason.
- Backend request/runtime SQL, HTTP, MCP, live-stream privacy, replay, version,
  and retry regressions pass. Full race/coverage gate including CLI tests is
  84.6% overall; app 91.7%, artifacts 90.8%, auth 90.2%, HTTP 92.0%, runner 91.6%.
  Staticcheck, eleven Semgrep rules, Gosec (zero findings/suppressions), Go/npm
  advisory checks, and Linux/amd64 static cross-builds pass. Reporter filesystem
  findings were repaired with scoped roots and checked integer conversions.
  Independent open attention is now summarized and deduplicated against its
  corresponding task blocker. Frontend and deployed acceptance remain pending.
- Verified native candidate is installed locally with readiness HTTP 204; the
  previous verified web assets remain until frontend QA. Build provenance is
  explicitly a dirty feature candidate, not a clean committed release. Both
  actual coding participants reported their own native sessions through narrow
  one-day, single-project credentials, without invented runs or conversations.
  Root used MCP to claim this actual Sentry release task and post the actual
  backend test evidence, then created a named peer review request. Peer response,
  grouped frontend adoption, and final reconciliation are still in progress.
- Final review found the legacy runtime event compatibility path could bypass
  typed actor ownership checks. Ordinary runtime source credentials are being
  restricted to typed agent/session/run observations; legacy compatibility
  requires explicit privileged source:admin. Regression/full QA rerun is pending.
  This does not change the explicit reporter or Sentry publisher contract.
  The unwired Hermes codec also needs resource-kind-based native subject
  selection so task metadata cannot relabel a session/run event.
- Runtime and audit privacy repairs are implemented with unit/SQL/API
  regressions; final consolidated backend QA is rerunning. New coordination
  audits omit request/response payload duplication, and Get/List safely redact
  existing audit details without mutating history. Hermes native event subjects
  and nonsecret HTTP error messages are corrected. The actual peer response is
  recorded, with evidence and explicit limits on automatic hooks/execution.
- Frontend validation has reached 135 tests and 28 native Chromium/WebKit
  checks. Screenshot review prompted a final project-label readability
  adjustment; request response evidence and closure outcome are included in
  existing desktop/XR inspectors. Final frontend gates and installation remain.
- Final backend gate after all review repairs passes: 84.7% overall, app 91.6%,
  artifacts 90.8%, auth 90.2%, HTTP 92.0%, runner 91.6%. Runtime-only credentials
  reject generic protected-resource mutations; historical audit bodies remain
  private without changing stored history. Staticcheck/Gosec/Semgrep pass.
  OSV and npm advisory checks report no affected packages; Go call analysis
  reports no reachable vulnerability. Artifact fail-closed checks also pass.
- Final frontend is verified and installed: 137 tests; line coverage 95.07%,
  branches 84.12%, statements 93.06%, functions 91%. Pinned type/lint/strict API
  checks and production build pass. Chromium/WebKit 28 checks plus the affected
  four final typography/layout checks pass. Old hashed assets remain available
  for open tabs. Final native security candidate is running with readiness 204.
  Live read-only checks deny source request reads, outside-project runtime access,
  and public bearer use with 403. Actual HTTPS portfolio acceptance is next.
- Actual HTTPS portfolio acceptance passes in both engines: 99 authorized
  summaries, four real regions, twelve-project disjoint pages, retained return
  position, actual peer response/evidence, mobile layout, CSP, and healthy live
  sockets. The peer request is resolved at version 3; owned task is in review.
  Final source/history secret inventory and all eleven Semgrep rules pass.
  `796c235` clean native/Linux/web builds and local activation are verified.
  Temporary reporting is ended and acceptance credentials revoked/removed.
  Current local release checklist is complete; full device and remote promotion
  acceptance remain separate, with no automatic native hook/execution claim.
