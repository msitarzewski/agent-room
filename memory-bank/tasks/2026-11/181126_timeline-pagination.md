# Timeline pagination and event-type filtering

On November 18, 2026, opening the Timeline for a high-volume observed project hit the request rate limit while fetching project history. Michael approved a pager, an event-type dropdown, and reverse chronological order. The changes were applied to `feat/shared-host-migration`.

## Behavior

- Timeline opens on the 50 most recent occurrences. Older and Newer follow cursor boundaries. Events with identical occurrence times use the higher ingestion cursor first. A late delivery appears at its occurrence time. The PostgreSQL index supports this order. See `internal/postgres/repository.go:561` and `db/migrations/0002_timeline_order.sql:2`.
- The Event type dropdown defaults to All event types and includes types from the operator's visible history across the project. Selecting a type combines an exact match with the existing text search and returns to the first page. See `web/src/pages/TimelinePage.tsx:94` and `internal/postgres/repository.go:645`.
- Complete live events update the first page in occurrence order. Older pages stay in place as new activity arrives. See `web/src/pages/TimelinePage.tsx:154`.

## Local verification

- All 147 Vitest cases and all 38 Chromium and WebKit browser cases passed. A 9,110-event fixture opened in one history request and returned 50 rows. Cases covered a timestamp tie across a page boundary, filtered results, late arrivals, project changes, and access revocation. See `web/src/test/App.test.tsx:506` and `web/tests/browser/control-room.spec.ts:231`.
- Chromium and WebKit both exercised the locally built application over HTTPS with 9,112 stored events. The full Go suite passed with race detection, including PostgreSQL and API contract checks. Backend coverage was 85.2%. Go vet, TypeScript, ESLint, OpenAPI validation, the production build, and `git diff --check` passed.

## Deployment

Release `086d31a3a9a97956da93ada6dcdefff76dae07f3` went live on the shared host at 18:58 CST on November 18. The localhost readiness check returned HTTP 204, the Timeline index was present after migration, and the web asset digest served over the tailnet matched the signed release.

A signed-in Safari session displayed the pager and Event type dropdown. The production API for that observed project returned 50 events on the newest page and 50 on the next older page. Selecting `task.observed` returned all seven matching events. The latest project observation occurred at 18:59:26 CST.
