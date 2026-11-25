# 241126_observability-and-ops-hardening

## Objective
Address the highest-impact operational and observability issues observed after the shared-host cutover: Sentry `partial` coverage false positives, `project.observed` event amplification, silent service-token expiry, non-destructive migration health checks, repeatable shared-host upgrades and backups, and routine-monitoring copy in the Attention brief.

## Outcome
- Go: `go vet ./...` and `go test -race ./...` pass across all packages.
- Python: 24 Sentry collector tests, 30 GitHub collector tests, and 86 deployment/migration tests pass.
- Web: `tsc`, `eslint --max-warnings 0`, 148 Vitest tests (93.64% statements / 95.72% lines), and production Vite build pass.
- Static/Security: `bash -n`, `shellcheck`, `tests/security/deployment-static.sh`, `tests/security/completeness-scan.sh`, and `tests/security/source-authorization-contract.sh` pass.
- Live Sentry verification after publisher reload: `scan_statuses` improved from `38 complete / 45 partial / 16 unreadable` to `83 complete / 0 partial / 16 unreadable`, with `0` pending, `0` quarantined, `0` auth errors, and `0` delivery errors delivered over the SSH tunnel to the shared host.

## Files Modified
- `adapters/source.py` — added `project_heartbeats` table, `enqueue_project_observation` (substantive fingerprinting, configurable heartbeat throttling, and offline pending-heartbeat coalescing), and top-level `auth_error_projects` status counter.
- `adapters/sentry/agentroom_sentry.py` — added `_unreadable_kept` to separate normal age-window retention from genuine file read errors, added `project_heartbeat_seconds` (default `600s`), and routed `project.observed` through `enqueue_project_observation`.
- `adapters/github/agentroom_github.py` — routed `project.observed` through `enqueue_project_observation(..., 0)` for offline pending-heartbeat coalescing without altering online poll cadence.
- `deploy/config/sentry-connector.toml.example` — documented `project_heartbeat_seconds = 600`.
- `internal/auth/manager.go` — added `ServiceTokenExpirySummary` and `SummarizeServiceTokenExpiry`.
- `cmd/agentroomctl/main.go` — extended `doctor` with active/expiring/expired service-token warnings and added `service-token expiring [window-days]`.
- `deploy/run-migration.py` — fixed `check()` to preserve `cutover-complete`/`retired` phases instead of clobbering `phase` to `checking` on transient errors.
- `deploy/shared-host.sh` — added post-cutover `backup` (verified online `pg_dump` for both databases, artifacts archive, SQLite backup, SHA-256 manifest) and `upgrade` (signed artifact verification, immutable release install, migration, atomic symlink flip, `/readyz` check with automatic rollback).
- `web/src/pages/AttentionPage.tsx` — improved singular/plural counts and calm wording for routine-monitoring-only briefs.
- `tests/sentry/test_agentroom_sentry.py`, `cmd/agentroomctl/main_test.go`, `tests/deployment/test_run_migration.py`, `tests/deployment/test_shared_host.py`, `web/src/test/App.test.tsx` — added unit and regression coverage for all changes.
