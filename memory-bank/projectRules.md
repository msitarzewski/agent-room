# Project Rules

**Last updated:** 2026-07-28

These rules supplement `AGENTS.md` with Agent Room-specific product and architecture constraints.

## Product Rules

1. Build for supervision of two to ten coding agents before expanding markets.
2. Prefer attention items and decisions over activity feeds.
3. Treat evidence as part of completion, not an optional attachment.
4. Keep the browser experience valuable without voice or spatial hardware.
5. Validate each milestone by supervisory work eliminated.
6. Separate chat participation from action authority.

## Architecture Rules

1. Keep adapters thin and provider-specific behavior outside the core domain.
2. Preserve native source and native ID for every imported fact.
3. Keep commands, events, projections, messages, and raw telemetry distinct.
4. Make commands idempotent and events immutable.
5. Separate persistent agent identity from host, instance, run, and session.
6. Keep raw telemetry out of the canonical coordination event stream unless promoted to a semantic event.
7. Use open standards where they fit; ARP adds coordination semantics rather than replacing telemetry standards.
8. Make every projection rebuildable.

## Safety Rules

1. Public ingress never implies public authorization.
2. Destructive or irreversible actions always require attributable human approval.
3. Every mutating action records actor, policy decision, target version, and outcome.
4. Never place secrets in source, events, evidence metadata, logs, or the Memory Bank.
5. Development never receives reusable production write credentials.
6. Agent Room deployment automation never modifies Hermes installation or data.
7. Pip's chat presence grants no implicit control permissions.

## Deployment Rules

1. Build once and promote the verified artifact.
2. Keep development and production state isolated.
3. Only Caddy accepts public traffic; Agent Room listeners bind to loopback.
4. Keep administrative and debug surfaces private.
5. Back up before irreversible migration.
6. Retain and test a last-known-good rollback path.
7. Treat restore testing as part of backup implementation.

## Decision Rules

- Do not select a stack, identity provider, database, or deployment runner without an explicit decision record.
- Do not claim a capability from a product analogy before implementing and validating it.
- Mark uncertain facts as open decisions rather than filling gaps with assumptions.
