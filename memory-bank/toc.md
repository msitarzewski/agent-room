# Agent Room Memory Bank

**Last updated:** 2026-07-27

This index routes future sessions to the smallest useful context set. `boot.md` remains the founding product specification; the Memory Bank records working context, accepted decisions, implementation patterns, and operational procedures derived from it.

## Start Here

1. [quick-start.md](./quick-start.md) — minimum project orientation
2. [activeContext.md](./activeContext.md) — current state and pending decisions
3. [progress.md](./progress.md) — roadmap and completion state
4. [tasks/2026-07/README.md](./tasks/2026-07/README.md) — current month

## Recent Task Records

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

- [build-deployment.md](./build-deployment.md) — development-to-production contract for `host_agentroom` through `host_ingress`
- [decisions.md](./decisions.md) — accepted architectural and product decisions

## Update Policy

- Update `activeContext.md` at every state transition.
- Update the current monthly README for in-progress and completed tasks.
- Update `decisions.md` only when a decision is explicitly accepted.
- Update `systemPatterns.md` or `projectRules.md` only when implementation validates a reusable pattern.
- Do not duplicate detailed task history across files; link to the task record.
