# Spatial workspace — first release

**Updated:** 2026-10-15
**State:** COMPLETE — local deployment verified; Michael reports immersive mode works; remaining device acceptance pending
**Authorization:** Michael requested planning, implementation, continuous release documentation, repair of unfinished code, local deployment using MiradorXR's tailnet pattern, and the 2026-10-14 commit checkpoint. No release number has been assigned. Publication, signed production promotion, and live headset acceptance remain separate.
**Branch:** `feat/spatial-workspace`

## Objective

Build an original Agent Room visual workspace: portfolio → project → task → evidence on desktop, with immersive WebXR over the same authenticated control plane. Use real authorized records and preserve attention, provenance, review semantics, and safe commands.

MiradorXR is the technical reference for session entry, pinch handling, progressive detail, resource ownership, diagnostics, and desktop/XR continuity. Agent Room's visuals must be entirely new, inspired, calm, and magical. Motion communicates observed changes rather than fabricated work.

## Release checklist

- [x] Implement consistent authorized workspace snapshots and safe observation projection.
- [x] Implement complete paginated loading and reconnect-safe workspace refresh.
- [x] Build an original portfolio, project, and dependency scene with honest renderer fallback.
- [x] Implement WebXR entry, natural selection, exit, and desktop context continuity.
- [x] Integrate task, evidence, source freshness, and attention inspection with existing controls.
- [x] Verify functional, accessibility, browser, backend, and release checks.
- [x] Repair unfinished references and review the complete implementation diff.
- [x] Record final acceptance evidence and remaining device or deployment requirements.
- [x] Deploy the candidate locally through a dedicated Tailscale Serve HTTPS port.
- [x] Verify local tailnet login, workspace, live connection, and persistent service operation.

Checkbox text and this path stay stable so Sentry retains task identity. Sentry scans `memory-bank/releases/` as one release-progress card. All ten items are verified, including the final latest-source inspection repair. Physical headset acceptance and signed production promotion remain separate.

## Reuse and integration

- Extend `web/src/App.tsx:45` and `web/src/components/AppShell.tsx:6` with a workspace route.
- Extend `web/src/api/client.ts:80` for the workspace read contract and existing versioned, idempotent mutations.
- Extend `web/src/stream/StreamContext.tsx:45` and `web/src/hooks/useCollection.ts:8` for scoped refresh, cancellation, and pagination.
- Extend `internal/httpapi/server.go:97`, `internal/app/service.go:101`, and `internal/postgres/repository.go:232` for consistent snapshots.
- Extend `internal/postgres/projector.go:55` for typed source observations that preserve supervisor-owned fields.
- Follow `memory-bank/systemPatterns.md#Authority-and-Provenance` and `memory-bank/systemPatterns.md#Command-Event-Projection`.

New scene modules are needed because existing pages render HTML collections and contain no GPU renderer, XR lifecycle, or spatial layout. A snapshot hook is needed because separate collections cannot supply one transactionally consistent scene. A release file is needed because the existing task preserves design history and the monthly README is an index; Sentry has a separate release-progress parser for this directory.

## Acceptance

- Authorized projects occupy stable locations; selection exposes real tasks and dependency edges.
- A task opens evidence, associated runs, review state, source observations, and existing controls.
- Attention remains available at every scale. Unavailable or stale data is identified.
- Selection and layout never mutate canonical state. Commands preserve authorization, version checking, idempotency, and audit.
- Keyboard/list navigation remains useful when graphics or immersive sessions are unavailable.
- Renderer capability and XR errors are reported honestly.
- Tests cover loading beyond 50 items, project-switch races, reconnect refresh, observation ordering, permissions, selection, and layout.
- No production fixtures, invented counts, simulated workers, stubs, or unfinished implementation markers.

## Boundaries

The existing Timeline provides recorded activity. Full historical scene reconstruction requires additional snapshot/replay semantics; no scrubber will pretend current resources are historical state. Sentry's board, watcher, backups, and unresolved sync-order repair are not modified. Production worker execution is outside this visual release.

Vision Pro acceptance must check exact visionOS build, natural input, comfort, readability, and frame pacing. The user named 27.4 beta 4; exact build confirmation remains pending. MiradorXR's documented 88 fps is reference evidence, not an Agent Room measurement.

## Remaining device and promotion acceptance

2026-10-15: Michael reported, "Immersive works!" This confirms user-reported
immersive operation on the target headset. Exact model/OS build, detailed input
and inspection behavior, comfort, recovery, and measured frame pacing have not
yet been recorded. Preserve the completed implementation checklist above.

1. Record the actual Vision Pro model, visionOS build, Safari version, secure
   origin, and active renderer. Confirm session permission/cancellation and
   natural selection without requesting raw hand tracking.
2. Open a project, select a task, read full run-linked evidence and source
   references, reposition using the handle, and return to the portfolio. Confirm
   text size, reach, visual comfort, and dependency/attention clarity at real scale.
3. Exit/re-enter immersive mode and confirm the desktop project/task selection
   survives. Verify reduced motion, device interruption, and network recovery.
   Record measured frame pacing and resource behavior; no frame-rate promise is
   inferred from the reference project or headless browser tests.
4. Choose the product release identifier and committed revision to promote.
   The existing Linux signing builder then requires a clean committed checkout,
   signing key, and actual deployment configuration. Run its promotion gates on
   that exact revision. No production credentials or release ID were invented.

The current Mac deployment is local staging toward the operator-designated remote
machine. Verify that machine's platform before selecting and promoting its artifact.
Its actual host identifier stays in the private deployment record.

The subsequent [continuous Sentry release](continuous-sentry-observations.md)
installed an independent local publisher for the existing Agent Room project.
Michael's explicit 2026-10-15 approval expanded it to all 99 discovered projects
with full operator project permissions and source-only publisher access. Remote
connector deployment remains separate.

## Iteration log

### Local tailnet deployment — 2026-10-14

- Extended the existing native daemon/CLI, production web distribution, and
  PostgreSQL/Dex Compose service definitions. MiradorXR's loopback + Tailscale
  Serve HTTPS pattern is reused with dedicated app port 4445 and identity port
  4446; existing tailnet mappings are preserved.
- The per-user `local.agentroom` launch agent supervises the native service.
  Dedicated local containers keep application and identity databases persistent.
  Actual environment values and credentials stay in ignored `data/local/`.
  Development login and managed worker execution are disabled.
- Imported five actual repository task records through the private source
  contract; WebSocket updates reached the production UI without reloading.
  This one-time import does not install an automatic Sentry connector.
- A real HTTPS sign-in check exposed loss of the workspace destination. The
  existing App/AuthContext now preserve validated local path, query, and fragment
  through OIDC, while rejecting external destinations and login loops. Exact
  pinned type/lint/API, 111 tests, coverage, and production build pass.
- PostgreSQL, Dex, and the daemon were restarted after private database snapshots.
  Readiness, existing session/five-task persistence, deep-link login in Chromium
  and WebKit, and 24 native browser regression checks pass. Boundary verification
  confirms anonymous workspace 401, public adapter/MCP 404, development login
  disabled, and private readiness 204.
- Sustained stream verification exposed a WebKit compressed-frame protocol error
  after its first replay event; reconnections eventually occupy the socket limit.
  Chromium receives all five events and heartbeat without disconnecting. This
  real failure was repaired before local deployment acceptance. Both browser
  graphics paths visibly render after initialization.
- Isolated TLS A/B reproduced the current WebKit failure with context and
  no-context compression; disabled compression delivers all events and heartbeat.
  The control stream now disables compression and consumes control/close frames
  with a single reader. Transport closure no longer masquerades as lost access.
  Race/API regressions cover ping, peer close, unexpected client data, declined
  compression, and eight reconnects releasing socket slots.
- The source secret gate now scans a temporary inventory of Git-visible current
  source rather than intentionally private ignored runtime credentials. History
  scanning and credential rules remain intact. Disposable fixtures prove tracked
  edits and nonignored untracked leaks are caught, ignored runtime credentials
  are excluded, and deleted historical leaks remain detected. Actual source has
  zero findings; shell syntax and ShellCheck pass.
- Final consolidated backend quality passes with race/integration suites, 82.7%
  overall coverage, and all critical packages above 90% (HTTP 92.2%). Scoped vet,
  Staticcheck, Semgrep, and Gosec pass. Rebuilt native daemon/CLI and final static
  Linux/amd64 builds pass; the local launch agent runs the repaired daemon.
- Final real HTTPS Chromium and WebKit checks each receive five actual replay
  events and heartbeat over one open socket, with no unexpected application
  console errors or CSP violations. Deep-link OIDC, logout, secure context,
  secure HttpOnly cookies, and visible WebGL/WebGPU rendering are verified.
  WebKit screenshot capture injects a `body {}` stylesheet in Playwright's own
  animation-sync helper; the strict application CSP check excludes that capture
  action. The production policy remains unchanged.
- The local service stays running for Vision Pro Safari testing. Physical-device
  comfort, natural input, and frame pacing remain pending. No native visionOS
  package, commit, signed archive, or remote production activation was created.
- Final source refresh verified live source Done while supervised status remains
  inbox. It exposed missing latest-source description/title inspection: original
  human metadata is intentionally preserved, so its copied 8/10 checklist can
  differ from the latest 10/10 report. Existing desktop/XR inspectors now label
  and expose the full latest source observation; canonical human fields remain
  untouched. Snapshot refresh and XR pagination regressions pass. Exact pinned
  type/lint/API, 115 tests, coverage, production build, and 24 native browser
  checks pass; real HTTPS WebKit displays latest 10/10 beside preserved human
  metadata. The existing inspection checkbox is complete again.
- Sentry's read-only parser confirms this record is 10/10 Done. The final five
  actual source observations reached the local UI without a reload, retaining
  human descriptions/status and exposing latest report details. The temporary
  publisher token was revoked, rejected with 401, and removed. The operator login
  remains available in the private local credential file. Runtime metadata,
  source/binary digests, browser evidence, and restart snapshots stay ignored.

### Planning and baseline

- Startup and handoff loaded. Michael authorized build and ongoing documentation.
- MiradorXR reviewed at `a4b902ec`: Three.js 0.186.1, R3F 9.8.1, XR 6.6.31; 138 XR tests passed across 18 files.
- Feature branch created with preexisting security edits retained; baseline patch and hashes stored outside source.
- Backend, data, and scene ownership are independent; root integrates presentation, acceptance checks, licensing, and documentation.
- Inspected installed renderer sources: Fiber 9.8.1 assumes a WebGL XR manager API absent from Three's WebGPU XR manager. First release uses desktop WebGPU and a dedicated WebGL XR stage sharing scene state. This is an explicit compatibility decision, not a claimed WebGPU XR implementation.

## Validation evidence

Consolidated desktop, backend, security, and license checks pass. Implemented XR
lifecycle and interaction have automated coverage; the checked implementation
item does not imply headset acceptance or a measured Vision Pro frame rate.
The local acceptance record is complete. Native frontend gates use exact Node
24.8.0/npm 11.6.0; Linux Firefox verification uses the pinned image's Node
24.18.0/npm 11.16.0. Physical Vision Pro acceptance remains pending.

| Gate | Final result |
| --- | --- |
| Frontend | 115 tests; typecheck, zero-warning lint, strict API lint, and production build pass |
| Coverage | Frontend 92.17% statements / 82.24% branches / 89.15% functions / 94.61% lines; backend 82.7%, all critical packages above 90% |
| Browsers | Chromium/WebKit 24/24; Linux Firefox 12/12, final layout recheck, and headed software-WebGL rendering pass |
| Real integration | Eight-record isolated pilot plus five-record persistent local instance; ingestion, replay, catchup, manual transition, source preservation, attention, live refresh, and latest-source inspection pass |
| Security | Gitleaks, Semgrep, Gosec, Staticcheck, Go vet, govulncheck, npm audit, OSV, and authorization gates pass |
| Release material | Reviewed runtime licenses, static Linux/amd64 builds, emulated CLI execution, artifact fail-closed contract, and deployment-static pass |
| Completeness | No unfinished production markers, skipped/focused tests, unreviewed suppressions, or whitespace errors |
| Device / promotion | Actual Vision Pro acceptance and signed promotion pending; requirements recorded above |

### Implementation and real-data iteration — 2026-10-14

- `/workspace` now uses authenticated snapshots with repeatable-read consistency,
  complete collections, source receipt times, scoped stream refresh, and explicit
  stale or denied states. Portfolio reads have bounded concurrency.
- The new observatory has stable project geography, actual task dependencies,
  an accessible HTML directory/inspector, persistent attention, explicit review
  confirmation, and existing authorized command paths.
- Desktop selects WebGPU when available, falling back to WebGL2. Immersive XR
  uses its own WebGL stage with the same selected project/task state. XR store
  resources are owned by an effect and cleaned up across StrictMode/remounts.
- Visual review corrected initial graph framing, small labels, and an attention
  dock that obscured nodes. Production bundles exclude the development XR
  emulator and sample environments; real-headset entry remains present.
- A disposable real PostgreSQL/daemon pilot imported eight actual Sentry task
  records read-only from its checkout: four Upcoming, three Done, one Blocked.
  Ingestion, duplicate replay, catchup, live WebSocket delivery, and a manual
  supervised task transition were exercised. Source status stays separate from
  supervised status. No Sentry watcher, board, or backup was changed.
- The pilot exposed timestamp-sized source sequence overflow in a SQL expression;
  explicit bigint typing and regression coverage repair it. Subsequent review
  repaired trusted command replay, preserved source ordering during rebuild,
  and verified that stale source events cannot overwrite human changes.
- Blocked source observations create deduplicated source-owned attention; newer
  unblocked observations resolve only that attention. A source's Done status
  never implies accepted completion.
- Earlier native backend race/integration and frontend/browser checks passed;
  the final consolidated rerun includes the new regressions and visual changes.
- Runtime dependency license texts and a version/digest manifest have been
  added; the complete license gate passes. Linux/amd64 daemon and CLI builds are
  static x86-64 ELF binaries. A signed production archive still requires the
  operator-assigned version, clean approved commit, Linux builder, and signing key.
- Final frontend gate: 110 tests pass; coverage is 92.26% statements, 81.93%
  branches, 89.34% functions, 94.81% lines. Typecheck, zero-warning lint, strict
  OpenAPI, and the production build pass. Graphics chunks remain lazy and large
  (approximately 300 KB scene and 200 KB WebGPU backend, gzip); this is recorded
  without weakening the build warning threshold.
- Chromium/WebKit acceptance passes 24 checks, including actual renderer startup,
  evidence navigation, keyboard access, reduced motion, axe accessibility, and
  desktop/mobile workspace layout. The real pilot exposed a mobile navigation
  intrinsic-width overflow; a bounded grid track fixes it with browser regression.
- Diff review found the existing collection cursor selected the excluded
  lookahead row, skipping a record per page. It now uses the last included row;
  123 equal-timestamp tasks traverse every page exactly once in regression
  coverage. The static gate's unused test response now has a meaningful failure
  diagnostic, and pinned Staticcheck passes.
- XR inspection now includes run-linked evidence/artifacts without duplicating
  dual-linked records, with paginated full source references. Both desktop and
  XR disclose missing dependency details and tasks in/downstream of cycles.
- Completeness, deployment-static, source authorization contract, and diff
  whitespace checks pass. Gitleaks and Gosec report zero findings; the complete
  security/coverage/vulnerability rerun passes after the final corrections.
- The fresh vulnerability database exposed 11 reachable standard-library
  advisories fixed in Go 1.26.9 and GO-2026-6629 in `x/text` fixed in 0.41.0.
  Both patch updates are applied, extending the previous security repair; no
  advisories are suppressed. The license checker needed rebuilding with the
  same Go runtime to recognize its new standard-library directory. Final gates
  pass with the patched toolchain.
  Sources: [Go release history](https://go.dev/doc/devel/release#go1.26.9),
  [text advisory](https://pkg.go.dev/vuln/GO-2026-6629).
- Consolidated patched security gate passes: Gitleaks history/tree zero findings,
  Semgrep nine rules / 81 files zero findings, Gosec zero issues, Staticcheck and
  Go vet clean, govulncheck zero reachable vulnerabilities, npm audit zero, and
  OSV zero affected called packages. Uncalled module advisories remain visible
  in scanner output; no ignore list was introduced.
- Full race/integration/API coverage gate passes: backend 82.7%; app 92.1%,
  artifacts 90.8%, auth 90.2%, HTTP 92.4%, runner 91.6%. The complete license gate
  passes after rebuilding the pinned audit tool with Go 1.26.9. Artifact
  verification rejects missing integrity evidence and emits the exact version
  contract in its isolated harness.
- Real production UI check displays all eight Sentry tasks with no page errors
  or horizontal overflow at desktop/mobile widths. Pilot daemon, temporary login,
  sessions, and source token are stopped/revoked; credential files are removed.
  Sanitized pilot evidence remains outside source. Sentry itself was read-only.
- Exact pinned frontend environment also passes every source gate: official
  checksum-verified Node 24.8.0 / npm 11.6.0 on native darwin/arm64, 110 tests,
  matching coverage, clean type/lint/API checks, and production build. No
  dependency reinstall or source pin changes were needed. The Go 1.26.9 static
  Linux/amd64 CLI executes successfully under emulation in the digest-pinned
  Playwright Linux image; it reports an unversioned development build honestly.
- Final real-data visual review exposed overlap when a dependency layer wrapped.
  Wrapped columns now reserve their full width, partial columns are centered,
  and camera depth/zoom scale with graph bounds. Regression verifies eight
  same-layer tasks and the following layer's label separation. Native gates and
  browser checks pass after this correction.
- Linux Firefox passes all 12 checks. Final-layout rechecks pass in headless
  fallback and headed Xvfb software WebGL, with visible geometry, dependency
  threads, halos, and labels. This verifies the third browser engine without
  claiming hardware GPU or headset performance. Disposable containers and
  Linux node_modules were removed; source hashes match the reviewed checkout.
- Disposable PostgreSQL and pilot processes are removed. No application commit,
  PR, signed archive, or production deployment was created. The complete local
  candidate remains on `feat/spatial-workspace` for human/device acceptance.
- Sentry's read-only scanner recognizes this stable release record as 8/8
  complete (`TaskStatus.DONE`). The monthly visual-workspace card remains
  IN-PROGRESS for device acceptance and promotion; normal watcher delivery owns
  board synchronization.

## References

- [Design handoff](../tasks/2026-10/zoomable-control-room-plan.md)
- [October tracking](../tasks/2026-10/README.md)
- [Active Context](../activeContext.md)
