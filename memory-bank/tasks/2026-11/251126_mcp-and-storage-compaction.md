# 251126_mcp-and-storage-compaction

## Objective
Expand the private MCP agent workspace surface, eliminate unbounded `project.observed` / `event_outbox` PostgreSQL growth while preserving append-only history invariants, and surface service-token expiry health directly in the browser control room.

## Changes
- **Durable source receipts & heartbeat deduplication** (`db/migrations/0003_source_receipts.sql`, `internal/postgres/repository.go`):
  - Added expand-only `source_receipts` table tracking per-(project, source) receipt and collection watermarks.
  - Updated `Ingest` so unchanged `project.observed` heartbeats (identical substantive fields, only `collected_at` newer) update the `resources` projection and `source_receipts` without appending redundant `events` / `event_outbox` rows. First observations and any substantive change still append canonical events.
  - Updated `projectSources` and `ComponentHealth` to read from `source_receipts` unioned with `events`/`event_outbox`.
- **Published outbox retention & CLI maintenance** (`internal/postgres/admin.go`, `internal/postgres/outbox.go`, `cmd/agentroomctl/main.go`):
  - Added `Repository.PrunePublishedOutbox(ctx, retentionDays, apply)` which backfills `source_receipts` before deleting already-published `event_outbox` rows older than `retentionDays` (never modifying append-only `events`, `command_results`, or `audit_records`).
  - Added hourly background outbox pruning (7-day retention) in `RunOutbox` and `agentroomctl maintenance prune-outbox [--retention-days 7] [--apply]` (dry-run by default).
- **Expanded MCP tools for agents** (`internal/httpapi/mcp.go`, `internal/httpapi/server_integration_test.go`):
  - Added `list_agents`, `get_project_overview`, `get_return_brief`, and `list_timeline` to the private project-scoped MCP server, reusing existing capability-enforced `Service` methods.
- **Browser service-token health** (`internal/postgres/repository.go`, `api/openapi/agent-room.v1.yaml`, `web/src/api/types.ts`, `web/src/pages/HealthPage.tsx`):
  - Exposed non-secret `service_tokens` counts (`active`, `expiring_within_14d`, `expired`, `status`) on `GET /api/v1/health/components` and rendered a **Service tokens** row on `/health`.
