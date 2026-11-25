# November 2026 Tasks

## Completed

### 2026-11-24 — Observability, Sentry coverage, token expiry, and shared-host ops hardening

- Fixed Sentry age-window `kept` classification so readable archived task files no longer trap healthy projects in `partial` / Coverage unknown, and added `project_heartbeat_seconds` throttling + pending-heartbeat coalescing + `auth_error_projects` reporting in `adapters/source.py` and `adapters/sentry/agentroom_sentry.py`.
- Added service-token expiry visibility to `agentroomctl doctor` and `agentroomctl service-token expiring [window-days]`.
- Fixed `deploy/run-migration.py check` to preserve `cutover-complete`/`retired` checkpoint phases, and added `backup` and `upgrade` modes to `deploy/shared-host.sh`.
- Refined Attention brief pluralization and quiet routine-monitoring copy in `web/src/pages/AttentionPage.tsx`.
- Verified Go (`go vet` + `go test -race ./...`), Python (24 Sentry + 30 GitHub + 86 deployment), Web (typecheck, ESLint, 148 Vitest, build), ShellCheck, and static/security gates. [Task record](241126_observability-and-ops-hardening.md)

### 2026-11-18 — Timeline pagination and event-type filtering

Deployed at 18:58 CST. Signed-in Safari displayed the controls. The production API for a high-volume observed project returned two 50-event pages and all seven `task.observed` events under the exact type filter. [Timeline task record](181126_timeline-pagination.md)

### 2026-11-18 — Shared-host cutover recovery

- Cutover completed at 18:22 CST. Both databases were restored, GitHub delivered from the destination, and Sentry delivered for all 99 projects through the Mac's SSH tunnel. Browser acceptance and local retirement remain pending.
- All 83 deployment tests and 53 collector tests passed in the deployment checkout. [Recovery record](181126_cutover-recovery.md)

## Earlier migration work

### 2026-11-17: Shared-host migration and source recovery

- Michael reaffirmed remote deployment as the goal. State: APPLY / WAITING_TOOL,
  working on `feat/shared-host-migration`.
- The local app/login/99-project walkthrough passes. The destination is staged
  but inactive; its isolated PostgreSQL service alone is running.
- Replaced the expired Sentry ingestion token at Michael's request with exact
  end-of-day December 31, 2026 Central expiry. Original actor, 99 project scopes
  and ingestion-only capabilities are preserved; old token revoked.
- Preserved 101,973 immutable queue records, took a consistent private backup,
  and rescheduled only failed authentication retry timers under the process
  lock. All 99 delivery errors cleared; pending and quarantined are both zero,
  with fresh successful delivery for every mapped project.
- Added remote staged-manifest/signature/privilege preflight before any local
  shutdown. It runs reviewed code inline against the old staged bundle, fails
  before fencing, and preserves uncertain-activation recovery behavior.
- Remote sudo requires interactive operator input under the canonical host
  guide. Actual cutover, remote browser acceptance and local retirement remain
  pending. No password belongs in chat or source control.
- Michael explicitly requested assistant execution. The reviewed cutover is now
  running in a dedicated native Terminal window (`Agent Room migration`) so he
  can enter sudo directly. The preflight reached sudo before any new local
  shutdown; final export/activation remain unattempted at the last check.
- 76 migration tests and 53 collector tests pass (129 total), plus bundle,
  shell/static/completeness checks. Prepared one exact interactive cutover
  command, retaining old staged hash binding and requiring fresh final exports.
- See [current context](../../activeContext.md#destination-cutover-continuation--2026-11-17)
  and [deployment procedure](../../build-deployment.md#shared-host-migration).
