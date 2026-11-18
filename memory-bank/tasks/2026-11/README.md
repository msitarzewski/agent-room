# November 2026 Tasks

## In Progress

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
- 76 migration tests and 53 collector tests pass (129 total), plus bundle,
  shell/static/completeness checks. Prepared one exact interactive cutover
  command, retaining old staged hash binding and requiring fresh final exports.
- See [current context](../../activeContext.md#destination-cutover-continuation--2026-11-17)
  and [deployment procedure](../../build-deployment.md#shared-host-migration).
