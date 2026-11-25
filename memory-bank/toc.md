# Agent Room Memory Bank

**Last updated:** 2026-11-24

This index routes future sessions to the smallest useful context set. `boot.md` remains the founding product specification; the Memory Bank records working context, accepted decisions, implementation patterns, and operational procedures derived from it.

## Start Here

1. [quick-start.md](./quick-start.md) — minimum project orientation
2. [activeContext.md](./activeContext.md) — current state and pending decisions
3. [progress.md](./progress.md) — roadmap and completion state
4. [tasks/2026-11/README.md](./tasks/2026-11/README.md) — current month

## Recent Task Records

- [Observability and ops hardening](./tasks/2026-11/241126_observability-and-ops-hardening.md) — Sentry coverage/heartbeat throttling, CLI token-expiry checks, Attention brief wording, and shared-host backup/upgrade modes
- [Timeline pagination and event-type filtering](./tasks/2026-11/181126_timeline-pagination.md) — chronological paging, project-wide type selection, and verification
- [Shared-host migration](./build-deployment.md#shared-host-migration) — operator procedure, checkpoints, and acceptance criteria
- [November cutover recovery](./tasks/2026-11/181126_cutover-recovery.md) — local fencing, final upload, and recovery from interrupted provisioning

- [GitHub observations](./releases/github-observations.md) — merged read-only issue/PR/review/check source, local operation and return-to-interface handoff

- [Information layout follow-up](./releases/agent-coordination-workspace.md#information-layout-follow-up--2026-10-15) — deployed frontend `5ef2b30`, overlap/pagination/focus verification, clean-build provenance, and remaining device checks
- [Agent coordination workspace](./releases/agent-coordination-workspace.md) — approved agent-first request workflow, grouped portfolio, and shared desktop/XR coordination
- [Continuous Sentry observations](./releases/continuous-sentry-observations.md) — automatic collection, durable delivery, scoped setup, and activity/source freshness
- [First spatial workspace release](./releases/spatial-workspace.md) — approved scope, stable Sentry checklist, and ongoing validation evidence
- [Zoomable control room plan](./tasks/2026-10/zoomable-control-room-plan.md) — proposed visual workspace, Sentry observations, renderer investigation, and recovery steps
- [July 2026 tasks](./tasks/2026-07/README.md) — founding release and prior work
- [270726_founding-agent-room-release.md](./tasks/2026-07/270726_founding-agent-room-release.md) — implemented and verified founding control-plane release
- [270726_open-source-publication-readiness.md](./tasks/2026-07/270726_open-source-publication-readiness.md) — Apache-2.0 licensing, governance, redistribution, and privacy release
- [270726_product-foundation.md](./tasks/2026-07/270726_product-foundation.md) — mission, Memory Bank, Hermes Pip integration, and deployment foundation

## Product

- [projectbrief.md](./projectbrief.md) — mission, scope, goals, and non-goals
- [productContext.md](./productContext.md) — initial customer, jobs, differentiation, and measures

## Architecture and Engineering

- [systemPatterns.md](./systemPatterns.md) — architectural boundaries and reusable patterns
- [techContext.md](./techContext.md) — known technical context and open stack choices
- [database-schema.md](./database-schema.md) — conceptual entities and invariants
- [testing-patterns.md](./testing-patterns.md) — required test strategy
- [projectRules.md](./projectRules.md) — product and engineering rules

## Operations

- [build-deployment.md](./build-deployment.md) — local tailnet lifecycle and signed Linux/Caddy promotion contract
- [decisions.md](./decisions.md) — accepted architectural and product decisions

## Update Policy

- Update `activeContext.md` at every state transition.
- Update the current monthly README for in-progress and completed tasks.
- Update `decisions.md` only when a decision is explicitly accepted.
- Update `systemPatterns.md` or `projectRules.md` only when implementation validates a reusable pattern.
- Do not duplicate detailed task history across files; link to the task record.
