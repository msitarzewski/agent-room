# GitHub observations

**Status:** Locally verified; final CI and merge in progress
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
- [ ] Pass appropriate backend, collector, frontend and security checks.
- [ ] Merge the verified development PR and record deployed provenance.

## Verification and working state

QA / RUNNING on `feat/github-observations`. PR
[#7](https://github.com/msitarzewski/agent-room/pull/7) includes the unmerged
spatial/coordination prerequisite and GitHub observation support. Backend
checkpoint `5094f3d` is published; final consolidated checkpoint is pending.

- 53 collector tests pass: 30 GitHub and all 23 existing Sentry regressions.
- Backend vet/race/SQL/API gate passes: 84.9% overall; critical app 91.6%,
  artifacts 90.8%, auth 90.2%, HTTP 92.0%, runner 91.6%.
- Frontend 140 tests and 30 native Chromium/WebKit checks pass. A final focused
  pair verifies the Linux Firefox portability correction described below.
- Type/lint/strict API, Staticcheck/Gosec, Semgrep, source secret inventory,
  runtime redistribution/license, npm advisory and completeness gates pass.
  Go vulnerability analysis finds no reachable vulnerability; module advisories
  in unused code remain qualified by the existing call-analysis gate.
- Independent review fixes/regressions cover fail-closed native repository
  binding, report namespace ownership, partial-data retention, API backoff,
  pagination/reconciliation, multi-repository fairness, current-head SHA,
  carried-cycle freshness, bounded text and latest queued CI attempts.
- Actual public API collection delivered seven existing PR records with complete
  coverage and zero pending/quarantined deliveries. There are no standalone
  issue records in this repository at this check; deterministic regressions
  cover issue close/reopen and updates without fabricating production issues.
- Authenticated HTTPS inspection verifies real PR #7, its observed failed CI,
  current-head details, separate GitHub/Sentry freshness, and a connected live
  stream with no application/API errors. All 230 preexisting canonical task
  records retain the fields checked before ingestion. Public source bearer
  reads are denied; the private adapter exposes no task-read route.
- `local.agentroom-github` is supervised and polls every 900 seconds without a
  GitHub credential. Its Agent Room credential has exactly event:ingest and
  source:github for the mapped project. Static optional GitHub token files need
  external renewal; automatic App installation-token minting is not implemented.
- Existing Sentry publisher was reloaded with shared delivery: 99 projects,
  zero backlog/quarantine and no collection/transport error at the recorded
  check. Its own board/watcher/history/backups and source configuration remain
  independently managed.

Initial PR CI passed backend, real daemon OIDC browser, security and CodeQL.
Linux Firefox correctly used the supported accessible workspace, exposing a
new overlap test that assumed a graphics renderer. The corrected test checks
separate necessary information and reachable decisions in either supported
mode; it does not change application behavior. Final rerun and merge are pending.

The locally active native candidate has explicit dirty-build provenance; the
verified web assets remain `5ef2b30`. Private build/deployment records distinguish
artifacts. Current snapshots include both databases, native/web/configuration
and a consistent Sentry queue under `data/local/github/backups/pre-install`.
Stop the GitHub publisher before restoring an older daemon that lacks typed
GitHub support; retain its durable queue. After merge, clean merged builds,
activation, source merge-state reconciliation and final docs remain required.
