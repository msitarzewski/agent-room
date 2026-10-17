# GitHub observations

**Status:** Complete locally and merged
**Last updated:** 2026-10-17

## Objective and authority

Michael approved getting the read-only GitHub monitor working and merged, then
returning to the desktop/spatial interface. Agents remain the primary users.
Observe this repository's issues, pull requests, reviews and checks; retain
source links, native identities, freshness and uncertainty. Observation grants
no authority to comment, assign, approve, merge, execute work or change human
decisions. The user's merge instruction authorizes the development PR merge;
the deployed monitor itself has no GitHub write operations.

## Reuse and integration

Analyzed `adapters/sentry/agentroom_sentry.py:173` and its isolated tests:
durable queue, delivery and filesystem checks can be shared, while Sentry's
local scanner cannot retrieve GitHub API resources. A separate provider adapter
and provider tests are needed to keep GitHub credentials/API rules out of the
Sentry scanner. Extract shared source delivery rather than copy it. Extend
existing Go domain/ingest/projector/repository tests and typed observations;
preserve existing Sentry task IDs and source-only permissions.

The current project observation projection has one Sentry report per project.
GitHub needs a separate source report identity so its freshness never replaces
Sentry's. No new database schema or renderer is planned. This release record
follows the existing source/spatial release checklist pattern; the spatial
record cannot represent independent GitHub acceptance or source authority.

## Accepted scope

- Explicit mapping of this GitHub repository to its existing Agent Room project.
- Outbound GET requests only; anonymous public-repository monitoring initially.
  Optional protected token file supports repositories requiring authentication;
  no implicit reuse of the development CLI's broader credential.
- Durable, source-scoped identities, deduplication, retries and reconciliation.
- Issue close/reopen, PR draft/open/review/close/merge, attributed review facts,
  and check/status observations tied to the current head commit.
- Rate limits, pagination, bounded requests, partial/unavailable data and source
  freshness remain explicit. Missing data never silently means success.
- Existing task/source/evidence/coordination views show attributable records;
  automatic agent execution and new interface design are outside this change.

## Acceptance checklist

- [x] Extend typed source ingestion, isolation and independent freshness.
- [x] Share durable source delivery while preserving Sentry regressions.
- [x] Implement and test read-only GitHub collection and retry/reconciliation.
- [x] Install scoped local source publisher and retain private rollback copies.
- [x] Verify actual issue/PR/check observations and existing source isolation.
- [x] Pass appropriate backend, collector, frontend and security checks.
- [x] Merge the verified development PR and record deployed provenance.

## Verification and operating state

[PR #7](https://github.com/msitarzewski/agent-room/pull/7) merged on 2026-10-17
as `582567d55717f85094950d3969c71831982b0d19`, after all exact-head checks passed.
The merge includes the previously verified spatial/coordination prerequisite.
The implementation/test checkpoints are `5094f3d`, `34c1ae7` and `68bea1e`.

- 53 collector tests pass: 30 GitHub and all 23 existing Sentry regressions.
- Backend vet/race/SQL/API gate: 84.9% overall; critical app 91.6%, artifacts
  90.8%, auth 90.2%, HTTP 92.0%, runner 91.6%.
- Frontend 140 tests, 36 native browser checks, and all 54 Linux
  Chromium/Firefox/WebKit CI checks pass. Dense viewport cases independently
  verify graphical/accessible layouts under the normal deadline. The CI fixes
  change tests only, preserving application behavior and all layout assertions.
- Type/lint/strict API, Staticcheck/Gosec, Semgrep, CodeQL, source/history secret
  scans, redistribution/license, npm/OSV, completeness and source boundaries
  pass. Go call analysis finds no reachable vulnerability; advisories in unused
  module code remain explicitly qualified by that gate.
- Real daemon OIDC browser, reproducible release package, fail-closed provenance
  and deployment-configuration CI checks pass.
- Independent review findings are fixed/regression-tested: repository binding,
  report ownership, partial-data retention, backoff, pagination/reconciliation,
  budget fairness, current-head/combined-status SHA, cycle freshness, bounded
  context and newer queued CI attempts. No unresolved review findings remain.
- Real public API collection delivered seven existing PR records. No standalone
  issues exist at the recorded check; deterministic tests cover issue lifecycle
  without fabricating production issues. Actual PR #7 exercised draft,
  failed-CI source blocker, new-head review and merged state. Only source-owned
  attention resolved; canonical task state remains inbox. All 230 preexisting
  canonical task records retain their checked fields.
- Authenticated HTTPS after activation verifies merged PR details, independent
  GitHub/Sentry reports and a connected socket without application/API errors.
  Public source-bearer reads are denied; the private adapter exposes no task-read
  route. GitHub API requests are GET-only and the public deployment uses no
  GitHub credential. Optional protected read-token files need external renewal;
  automatic App token minting is outside this monitor. It collects review
  decisions; individual review-comment threads/resolution are outside this scope.
- Both supervised publishers are healthy at final checks: GitHub one project,
  Sentry 99, zero pending/quarantined events and no collection/transport error.
  Source ingestion never changes ownership, accepts completion or executes work.

## Deployment and recovery

Clean native Darwin/arm64 and static Linux/amd64 daemon/CLI builds record the
exact merged revision with vcs.modified=false. The pinned clean frontend build
matches all nine verified installed files byte for byte. Native binaries and web
provenance are activated locally; readiness is 204 and all nine files verify
through HTTPS. Exact hashes and current phase are private in
`data/local/github/logs/merged-build.json`, `data/local/github/final-state.json`
and `data/local/deployment.json`. A subsequent documentation-only handoff may
advance main without changing this verified deployed source revision.

`local.agentroom-github` polls every 900 seconds with an exact single-project
Agent Room token containing only event:ingest/source:github. Its GitHub API
access is anonymous public read-only. Private config, credential metadata, queue,
logs and evidence stay ignored in `data/local/github/`. Sentry was reloaded with
shared delivery; its separate board/watcher/history/backups/configuration remain
independently managed. No public route or tailnet mapping was added.

Pre-install snapshots include both databases, native/web/configuration and a
consistent Sentry queue under `backups/pre-install` in that private directory.
`backups/after-acceptance` contains consistent copies of both source queues.
Stop the GitHub publisher before restoring an older daemon without typed GitHub
support. Preserve its durable instance/repository bindings and queue for recovery.
The disposable integration-test database/container was removed.

## Next work

Return to the app's agent-first desktop/spatial interface as Michael requested.
Use genuine review/check/issue context without making the human the dispatcher.
Physical Vision Pro interaction/comfort/frame pacing, deliberate real runtime
onboarding and verified pipx promotion remain separate. No autonomous executor,
merge-eligibility inference or remote activation is claimed.
