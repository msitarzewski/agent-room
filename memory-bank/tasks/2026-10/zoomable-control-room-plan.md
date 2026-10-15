# Zoomable control room plan

**Recorded:** 2026-10-13
**State:** APPROVAL
**Substate:** IDLE
**Implementation:** First local release candidate verified; human/device acceptance pending
**Authorization:** Michael subsequently authorized planning, building, continuous release documentation, and repair of unfinished code. The original exploration below is retained as design history. Current scope and evidence live in [the release checklist](../../releases/spatial-workspace.md). Deployment remains separate.

## Objective

Explore an Agent Room interface that presents projects, tasks, workers, attention,
and evidence in one zoomable visual workspace. Preserve the existing control
plane and use Sentry as a source of repository and task observations.

Michael questioned whether a third-party board should shape Agent Room's
experience and proposed a WebGPU dashboard with animated objects, graphing,
real-time infographics, and zooming into projects to reveal their tasks. He
asked to move the next phase into this repository and save the plans for recovery.

## Proposed product direction

Agent Room should own the primary supervisory experience. Sentry supplies
observations; Vikunja remains an optional planning connector during a pilot.
Existing source systems retain authority for the facts they originate.

This follows boot.md:40, boot.md:138, boot.md:178, and boot.md:695, plus
memory-bank/systemPatterns.md#Layered-Control-Plane-Architecture and
memory-bank/systemPatterns.md#Authority-and-Provenance. The current product
context describes an attention inbox as the primary surface; adopting a
zoomable workspace would refine that interaction model while keeping attention
available at every scale. Reconcile the specification and product context after
the interaction direction is approved.

## Semantic zoom

### 99-project immersive feedback — 2026-10-15

Michael reports immersive mode works, but the populated portfolio is a "parade
of icons" with no meaningful grouping, urgency, or asks. Proposed next iteration;
Michael approved proceeding with this agent-first iteration on 2026-10-15.
Implementation and verification are tracked in
[Agent coordination workspace](../../releases/agent-coordination-workspace.md).

Michael then clarified: "this project is really for agents. I'm a passenger
here. mostly." This supersedes the human-centered Needs You emphasis of the
first response. Primary participants are agents coordinating work; the human
mostly watches, occasionally joins, steers, or authorizes an exception.
Machine-readable records/tools are the agents' workspace, and XR is a window
onto the same coordination state. Repository inventory is supporting context.

Observed implementation gaps:

- `web/src/workspace/scene.ts:50` places projects by identity; at
  `web/src/workspace/scene.ts:117` their default color is uniform, with an amber
  exception based only on attention passed into the scene.
- `web/src/workspace/scene.ts:175` suppresses labels above 16 nodes unless they
  are selected, hovered, or carry attention, leaving most of the 99 anonymous.
- `web/src/pages/WorkspacePage.tsx:175` supplies only the current project's
  resource snapshot. Its existing portfolio summaries are confined to the HTML
  directory. HTML attention at `web/src/pages/WorkspacePage.tsx:170` is outside
  the XR world, whose controls are at `web/src/workspace/SceneCanvas.tsx:449`.
- A read-only local aggregate found one open attention item and one
  source-blocked task, no running runs, pending approvals, or pending completion
  claims. These are observed counts, not UI fixtures or proposed fake data.

Proposed direction:

- Extend the existing authorized overview at
  `internal/postgres/repository.go:665` with explicit actionable signals rather
  than treating total resource counts as open attention/approvals. Reuse the
  existing bounded portfolio loader at `web/src/workspace/data.ts:30`; avoid
  fetching full task/approval collections for all 99 projects merely to color
  the scene. Preserve partial/denied/unknown data honestly.
- Named regions for observed work in motion, agent coordination waiting, and quiet/archived context.
  Quiet regions begin collapsed with names/counts and unfold on selection.
  Stable anchors and stable project positions preserve spatial memory.
- Reserve color for actual meaning: amber blocked/warning, violet explicit
  decision/review requests, teal recorded running work, dim blue-gray quiet
  known state. Critical attention receives a clear red badge; unknown/stale
  sources have a distinct label/outline. Pair color with text, shape, and count.
- An in-world coordination surface presents real agent requests, handoffs,
  dependency waits, and peer reviews with owner/recipient, reason, age, evidence,
  and allowed next action. A smaller Human Escalations surface contains only
  requests needing human authority or intervention. Reuse current Label/
  TaskInspector and safe command APIs. Selecting a request opens its actual
  agent/work/project/evidence context. Recipient and handoff behavior require
  explicit contracts; current attention records alone do not prove they exist.
- Sentry's Active/recent-Git classifications do not prove an agent is running.
  Source Done does not become human review/acceptance. Never invent asks to
  decorate a sparse scene. Separate source freshness from operational urgency.
- Extend persistent identities at `internal/domain/models.go:61`, run/session/
  task records, and normalized runtime events at
  `internal/adapters/normalize.go:30`. Existing Codex, Claude, and Hermes adapters
  supply integration foundations; actual live runtime wiring and agent request/
  handoff behavior still need verification. Do not represent all 99 repositories
  as active agents, fabricate conversations, or make the human relay routine asks.
- Extend the shared scene model for desktop and XR together, with real
  99-project acceptance, actionable-signal propagation, accessibility/labels,
  deterministic layout, loading races, and no-ask/unknown-state regressions.

No new service, graphics engine, inferred dependency graph, or release identifier
is needed. This refines the existing first release's supervisory usefulness.

Zoom changes the information an object reveals while retaining its identity
and position.

| Scale | Information and actions |
| --- | --- |
| Portfolio | Projects in stable locations, active workers, freshness, and decisions awaiting the human |
| Project | Tasks, real dependency links, assignments, blockers, and recent changes |
| Task | Execution history, worker activity, source status, review state, and permitted interventions |
| Evidence | Inspectable diffs, tests, artifacts, source references, and approvals |

Start with a mostly planar scene with selective depth, readable labels, and
smooth transitions. Keep project positions stable. Collapse quiet regions
when needed and reveal detail progressively instead of moving every node
whenever events arrive. Preserve breadcrumbs, direct links, keyboard navigation,
reduced motion, and the existing list/detail views.

Animation conveys recorded state changes: an assignment changes a worker's
location, a blocked dependency highlights its affected chain, a completed run
enters review, and a disconnected source loses freshness. Avoid invented
completion percentages, inferred dependencies, and decorative motion that
suggests unobserved work.

Keep a visible attention queue at every scale. A later time scrubber should
reconstruct only the period covered by recorded events; it cannot recreate
past observations that were never captured.

## Architecture and reuse

- Extend the existing React/TypeScript application and route map at
  web/src/App.tsx:45. Add the exploratory view alongside existing routes.
- Reuse the reconnecting event stream and cursor handling at
  web/src/stream/StreamContext.tsx:45. A scene model should apply event updates
  separately from its animation loop.
- Reuse durable publication from internal/postgres/outbox.go:19 and the
  existing event/resource APIs in internal/httpapi/server.go:97.
- Preserve the existing task, run, session, source, evidence, and approval
  concepts in internal/domain/models.go:109.
- Keep inspectors, evidence, forms, and consequential controls in accessible
  React/HTML surfaces while using GPU rendering for the visual workspace.
- Connect Sentry through the private adapter listener at
  internal/httpapi/server.go:144, with the source-scoped ingestion checks at
  internal/app/service.go:585. Follow the encrypted-tunnel boundary documented
  in README.md:266.
- Preserve authenticated, version-checked commands for mutations.
  Scene animation and selection never substitute for a control-plane command.

No replacement backend, custom graphics engine, or production worker service
is part of the proposed first prototype. Existing architecture and reusable
services take priority over new modules.

## Renderer investigation

The next design spike should compare the interaction on the actual target
devices before selecting a renderer.

- Three.js WebGPURenderer supports WebGPU with automatic WebGL 2 fallback.
  Its official documentation still describes it as experimental.
- PixiJS supports WebGPU and WebGL; its official guide currently recommends
  WebGL for production.
- Browser/GPU capability detection, device loss, fallback behavior, text
  readability, picking, frame pacing, and power usage belong in the spike.
- Measure performance using real observations at the current project/task
  scale. Rendering at a high frame rate does not make source data fresher.

References checked 2026-10-13:
[Three.js WebGPURenderer](https://threejs.org/manual/pages/webgpurenderer),
[PixiJS renderers](https://pixijs.com/8.x/guides/components/renderers), and
[MDN WebGPU](https://developer.mozilla.org/en-US/docs/Web/API/WebGPU_API).
Recheck current support before dependency selection.

## Sentry findings to carry forward

The sibling Sentry checkout was reviewed at commit 366a979 on feat/git-history.
That is its first full-project checkpoint. It has no remote, and nothing was
pushed. Sentry's watcher and hourly history uploader were loaded; the uploader's
last exit was successful. These are observations from 2026-10-13.

A read-only dry scan found 97 projects and 208 task records in 4.49 seconds,
using 461 Git calls. The Sentry test run executed 94 tests: 90 passed and four
integration tests were skipped. Agent Room's own application tests were not
rerun during that review.

1. Partial sync recovery needs repair. In Sentry's sentry/sync.py:239, the
   source-status description is updated before the column move. If that move
   fails, the next sync can report unchanged and leave the card in the wrong
   column. An isolated fixture reproduced this. The gone-label/move sequence
   at sentry/sync.py:306 needs the same recovery review.
2. Task identity needs to survive wording and document changes. Sentry's
   sentry/scan.py:405 hashes item text, and sentry/sync.py:111 includes the
   source path. Two source records currently represent the watcher log-noise
   task. Consider optional explicit task IDs with aliases that preserve
   existing cards, human notes, and manual placement.
3. Collect once and deliver independently. Sentry's sentry/__main__.py:121
   captures Git history before board access, but task collection follows board
   access. Move observation collection earlier and evaluate a durable delivery
   queue, extending the SQLite store at sentry/history.py:31.
4. Preserve source status separately from supervised work state. Sentry marks
   dated task write-ups Done at sentry/scan.py:430. Imported documentation must
   not silently become reviewed completion or overwrite a human assignment.
5. Add source freshness to the portfolio. Sentry currently uses a 60-second
   debounce and 15-minute periodic scan at sentry/watch.py:17. Immediate worker
   signals require lifecycle adapters. Agent Room's component health currently
   records adapter last-seen timestamps but labels adapter status ok at
   internal/postgres/repository.go:92.

The review could not retrieve the Vikunja token in that session. Live board API
contents were not audited. No Agent Room listener was reachable at the local
documented ports; a remote production deployment was not verified.

## Integration gaps at the planning handoff

Private ingestion and source-event deduplication already exist:
internal/httpapi/server.go:144, internal/app/service.go:585, and
internal/postgres/repository.go:484. Typed repository observations still need
design work. The current projector excludes project resources at
internal/postgres/projector.go:252 and defaults generic task events to working
at internal/postgres/projector.go:86.

The first release now implements the typed Sentry contract, stable task identity,
source ordering, protected supervisory fields, and a real-data ingestion pilot.
The descriptions in this section record the original gaps; use the
[release record](../../releases/spatial-workspace.md) for current status.

Define source-specific schemas, stable project/task mappings, ordering,
deduplication, and ownership of writable fields before importing observations.
Keep source-reported Done separate from evidence review and accepted completion.
Vikunja edits can later trigger reconciliation through webhooks, but periodic
reconciliation remains necessary because failed deliveries are not retried:
[Vikunja webhooks](https://vikunja.io/help/webhooks/).

Production worker execution is separate work. The current configuration rejects
in-process managed runtimes at internal/config/config.go:340; README.md:273
records that no production connector service or isolated runtime worker ships
with this foundation.

## Proposed first prototype

1. Agree on the first interaction and acceptance scope: portfolio to project
   to task, with attention and an evidence inspector.
2. Resolve existing branch work before implementation; use an isolated branch
   or worktree without discarding the pending security changes.
3. Use Sentry's own project for the first observation-integration pilot and
   real project/task records for evaluating the overview. Keep access controls
   intact across projects.
4. Prototype stable layout, semantic zoom, meaningful transitions, source
   freshness, and recorded-event playback alongside the current UI.
5. Compare renderers and fallback behavior on the intended desktop and mobile
   browsers. Record measurements before selecting dependencies.
6. Exercise duplicate/out-of-order events, reconnect/catch-up, task renaming,
   manual overrides, permission changes, reduced motion, and keyboard access.
7. Review the prototype before expanding integration, changing the primary
   navigation, deploying, or enabling worker execution.

Success: identify active work, recent changes, blockers, and waiting decisions
within thirty seconds after an absence, with inspectable evidence available
from the selected task. This extends boot.md:929 and
memory-bank/productContext.md#Success-Measures. No renderer or latency target
has been selected yet.

## Existing repository work at the handoff

The checkout is on fix/weekly-security-scan. It already contained nine modified
tracked files before this handoff: .github/workflows/security.yml, README.md,
go.mod, internal/postgres/repository_integration_test.go,
memory-bank/activeContext.md, tests/security/deployment-static.sh,
tests/security/static-security.sh, web/package-lock.json, and web/package.json.
The security repair remains awaiting commit approval. Preserve it; do not
commit, revert, or replace those changes as part of this planning task.

## Recovery

Open the next session from this Agent Room checkout and read:

1. memory-bank/activeContext.md
2. memory-bank/tasks/2026-10/README.md
3. This planning task
4. memory-bank/systemPatterns.md and the relevant boot.md sections

Resume the state recorded in `activeContext.md` and
`memory-bank/releases/spatial-workspace.md`. Michael authorized the first release,
which now has a built local candidate, real Sentry ingestion evidence, and
desktop/browser/security checks. Exact physical Vision Pro acceptance and signed
production promotion remain separate; do not restart the original planning spike.

Suggested opening prompt: Startup, then read the spatial workspace release
record and resume its current acceptance/review state. Preserve the pending
security changes, including the new Go 1.26.9 and x/text patch updates.

## Artifacts

- This task, the October index, Active Context, and the Memory Bank index form
  the recovery handoff.
- The original handoff created no application code or deployment. The later
  user-authorized local candidate is documented in the release record.
- The filename intentionally omits a numeric date prefix because Sentry treats
  date-prefixed task write-ups as Done. The monthly PLANNED entry tracks this
  work without implying completion.
