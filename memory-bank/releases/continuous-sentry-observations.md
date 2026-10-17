# Continuous Sentry observations

**Updated:** 2026-10-15
**State:** DOCS / IDLE — local release complete
**Authorization:** Michael requested a Memory Bank update, commit checkpoint, and the next build after continuous Sentry delivery was identified as the next engineering step.
**Checkpoint:** `95f166b` — spatial observatory and local deployment, committed and rebuilt from a clean tree.
**Branch:** `feat/spatial-workspace`

## Objective

Continuously publish real Sentry project/task observations into the existing
Agent Room control plane, independently of Vikunja. Keep collection, durable
delivery, human supervision, and freshness distinct. Preserve the current Sentry
watcher, board, Git history, and backups.

## Release checklist

- [x] Reuse read-only Sentry collection with explicit configured project mappings.
- [x] Persist immutable observations and retry delivery safely across failures and restarts.
- [x] Reconcile operator-owned project setup and issue exact least-privilege publisher scopes.
- [x] Record typed project scan reports and expose collection time separately from receipt time.
- [x] Clarify project activity, connection state, heartbeat receipt, and idle elapsed labels.
- [x] Verify source isolation, zero-task freshness, duplicate delivery, offline recovery, and preserved human state.
- [x] Deploy the local publisher with real observations and document recovery and operation.
- [x] Commit the completed implementation and build the verified revision.

Keep this path and checkbox text stable for Sentry task identity. Physical Vision
Pro acceptance and remote production activation remain separate.

## Reuse analysis

- `internal/adapters/normalize.go` translates runtime events; it does not collect
  workstation repositories or persist observations while the server is offline.
- `internal/postgres/outbox.go` delivers events after server ingestion and cannot
  protect observations collected while that server is unreachable.
- Sentry's existing `config`, `scan.discover`, `scan.scan`, and `scan.read_tasks`
  already own source discovery and Markdown parsing. Reuse them read-only in an
  Agent Room-owned adapter instead of copying or modifying Sentry's parser.
- `internal/app/service.go` and `internal/postgres/projector.go` already enforce
  private typed source ingestion, stable task identity, and human field ownership.
  Extend them narrowly for project scan reports, including projects with no tasks.
- `cmd/agentroomctl/main.go` and `internal/postgres/admin.go` own privileged setup.
  Extend them for idempotent setup and exact multiple-project token scope.
- Existing stream, time, inspector, and test components supply the frontend
  extension points; no replacement frontend or control plane is needed.

The new Python adapter, focused tests, and configuration example are necessary
because no independent offline source collector exists. A separate release record
is necessary to preserve the completed spatial release's identity and evidence;
the monthly README remains the index rather than a second implementation log.

## Contracts

- Task external identity stays exactly `source + '#' + ident` within each mapped
  project, preserving the existing pilot task IDs. Source wording/path changes
  retain Sentry's existing identity behavior; no heuristic merges or deletions.
- Collector sequence starts above timestamp-sized pilot watermarks and persists
  monotonically. Queue event ID, payload, sequence, and observation time stay
  immutable through retries; acknowledge only successful ingestion.
- Missing/window-expired/unreadable records never imply deletion. Keep previous
  observations and report incomplete collection honestly.
- A typed `project.observed` event reports actual collection time and scan status.
  Source receipt time is separate; delayed delivery does not imply a new scan.
- Only the operator provisions projects and memberships. Publisher tokens have
  exact project scopes and source-ingestion capabilities, without wildcard,
  administration, resource replacement, or database credentials.
- Connector SQLite state, mappings, logs, and secret files stay in ignored private
  runtime storage. Polling defaults to 300 seconds; manual once/status commands
  support verification and diagnosis. Unknown projects require provisioning.
- The Mac remains staging; the operator-designated remote host remains the
  eventual home. No remote activation is part of this local build.

## Verification

Test initial collection, unchanged polls, changed tasks, source failures,
per-project authorization failures, shared endpoint outages, retry identity,
restart recovery, sequence adoption, zero-task project freshness, and unchanged
human supervision. Use isolated fixtures/databases for faults and real authorized
repository observations for the deployed acceptance check.

## Approved portfolio expansion — 2026-10-15

Michael explicitly approved: provision all discovered Sentry projects, give his
operator account full project permissions, and give the publisher source-ingestion
access only. This resolves the earlier automatic-review scope rejection.

Fresh discovery found 99 projects and 214 task records before the operational
documentation update. The existing CLI added 98 projects and reconciled full
operator memberships across all 99, preserving existing project IDs and names.
The same publisher now uses those exact project IDs and only
`event:ingest,source:sentry`, with a new 30-day credential. The previous credential
was revoked, rejected with 401, and removed. No new background process was added;
Sentry's board, watcher, history, and backups remain independent and unchanged.

Initial full-portfolio delivery collected 215 actual task records and delivered
all 99 project scan reports: 52 complete and 47 partial because Sentry retained
window-limited or unreadable records. There were no unreadable project scans,
pending events, quarantined records, unmapped projects, or delivery errors. Scan
completeness remains explicit rather than claiming all retained tasks were read.

Authenticated Chromium and WebKit checks each verify all 99 authorized project
cards and source reports, the full operator capability set, rendered portfolio,
and scoped project/task selection without application or API errors. Chromium
uses WebGL2 and WebKit uses WebGPU in these checks. A read-only comparison with
the pre-change database backup confirms all 13 original tasks retain their
human-owned fields; one additional real task arrived during collection.

Both databases and the durable queue were backed up privately before setup.
Exact publisher scopes and the two ingestion capabilities are verified;
out-of-scope private requests and public bearer-token requests are rejected with
403. Credentials, configuration, source paths, logs, and detailed evidence stay
ignored under owner-only `data/local/`; no runtime files are tracked. No source
code change, new release identifier, commit, push, or remote activation is part
of this operational expansion. Brief wording and remaining headset workflow/
comfort checks remain separate follow-up work.

## Iteration log

- Spatial checkpoint committed as `95f166b`; clean provenance verified in native
  daemon/CLI and Linux/amd64 binaries. Exact pinned production web build passes.
- Read-only discovery found 97 source projects and 214 task records across 20
  projects before this new release record. These are observed counts, not fixtures.
- Backend, collector, and frontend implementation have independent ownership;
  root owns provisioning, runtime lifecycle, integration checks, and documentation.
- Backend race/integration/coverage gate passes: 83.8% aggregate, all critical
  packages above 90%. Typed validation is at `internal/app/service.go:668`,
  source-only project reduction at `internal/postgres/projector.go:158`, and
  shared freshness at `internal/postgres/repository.go:637`.
- Exact pinned frontend gates pass: 121 tests, 24 native Chromium/WebKit checks,
  94.96% line coverage and 83.18% branches. Collector's 23 stdlib tests cover
  offline/restart retry, stable identities, private credentials, source failures,
  independent authorization failures, quarantine, and transaction rollback.
- Initial local delivery succeeds for the existing Agent Room project: 13 real
  task records, complete scan, no pending or quarantined events. Its independent
  launch agent is installed. Source-only token expires after 30 days.
- Automatic approval review rejected the proposed 97-project provisioning and
  operator access expansion. That action made no changes. The safer deployment
  uses only the existing project and changes no memberships. The remaining 96
  projects are reported as requiring provisioning; Michael's scope decision is
  pending. Broader portfolio activation is separate from this release.
- Real authenticated HTTPS Chromium and WebKit acceptance passes: one open
  socket with heartbeat, typed project observations, advancing collection times
  after publisher restart, no page reload, no CSP violation, stable 13 task IDs,
  and unchanged human fields. Chromium uses WebGL2; WebKit uses WebGPU here.
  These are browser checks, not physical headset acceptance.
- Native daemon and independent publisher remain supervised locally. Publisher
  logs are empty, last exit is 0, and durable queue has no pending/quarantined
  records. Lifecycle, token rotation, mapping, and recovery are documented in
  `memory-bank/build-deployment.md#local-sentry-publisher`.
- Source/history secret scans, 11-rule Semgrep, Gosec, full Staticcheck,
  completeness, deployment-static, API, and runtime license gates pass. The
  new Python suite runs in CI and local security checks. No unfinished production
  TODO/FIXME/STUB/TUB markers were found; future-time references are actual
  validation tests, and Python's future import is language syntax.
- The authorized final commit includes the implementation and current release
  documentation. Native and Linux/amd64 binaries plus the pinned web distribution
  are rebuilt from that clean revision; exact hashes and source revision belong
  in ignored `data/local/deployment.json`. No push, remote activation, or native
  visionOS package is part of this local release.

## References

- [Spatial release](spatial-workspace.md)
- [Active context](../activeContext.md)
- [October tasks](../tasks/2026-10/README.md)
- [Source authority](../systemPatterns.md#authority-and-provenance)
- [Source supervision](../systemPatterns.md#source-observations-and-supervision)
