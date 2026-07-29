# Conceptual Data Model

**Status:** Domain model, not a database migration

**Last updated:** 2026-07-28

PostgreSQL 18 is the selected storage engine. These entities and invariants
remain independent of physical schema details.

## Identity

### Organization

Boundary for users, projects, policies, and retention.

### Human

Authenticated person with roles and policy assignments.

### Agent

Persistent participant identity with name, role, capabilities, runtime home, voice/avatar presentation, and trust profile.

### AgentInstance

Concrete installation or process for an agent. Records host, runtime, version, presence, source-native ID, and last observation.

### Host

Infrastructure identity distinct from participants. The initial production
host is `host_agentroom`; Caddy and Agent Room are separate processes on that
host.

### Session

Conversational context with runtime, source-native ID, participants, parent session, start/end timestamps, and sensitivity.

### Run

Bounded execution associated with an agent instance, session, task, budget, status, and source-native ID.

## Work

### Task

Work item with owner, status, version, dependencies, priority, budget, review state, approval state, source, and timestamps.

### TaskTransition

Attributable transition request and result. Stores prior state, requested state, accepted state, reason, actor, policy decision, and causation.

### Claim

Worker assertion such as progress, blockage, or completion. Claims can be pending, supported, contradicted, verified, or rejected.

### Evidence

Reference supporting or contradicting a claim. Stores type, immutable locator or digest, native source, producer, capture time, sensitivity, and verification state.

### Artifact

Material output with media type, size, digest, storage locator, provenance, retention, and access policy.

## Event and Attention

### Command

Intent with actor, target, payload schema, idempotency key, expected version, authorization context, impact class, and outcome.

### Event

Immutable accepted fact with sequence, source, correlation, causation, sensitivity, and schema version.

### Situation

Correlated condition derived from events. Stores rule or detector version, confidence, severity, lifecycle, affected entities, and evidence.

### AttentionItem

Human-facing decision or notification with owner, urgency, deadline, explanation, recommended action, allowed actions, acknowledgement, and resolution.

### Approval

Attributable decision bound to an exact command or action version, with scope, expiry, result, and rationale.

### Intervention

Requested and executed control action with actor, target, policy decision, impact class, outcome, and resulting events.

## Policy and Operations

### Policy

Versioned authorization, approval, budget, notification, or automation rule.

### Budget

Limits and consumption for cost, tokens, time, turns, concurrency, or tools.

### AuditRecord

Append-only security and administrative record distinct from mutable application logs.

### Deployment

Artifact version, checksum, environment, migration set, actor, timestamps, health result, smoke result, and rollback relationship.

## Invariants

- IDs are globally unique within their entity namespace.
- Agent and host IDs cannot be interchanged.
- External facts retain native source and native ID.
- Commands are idempotent within a documented scope.
- Event order is stable within an aggregate or stream.
- Projections are disposable and rebuildable from authoritative records.
- Completion verification links to claims, evidence, policy, and reviewer.
- Approval binds to an exact action version and expires.
- Chat text cannot directly create an accepted command.
- Destructive or irreversible interventions require human approval.
- Secrets never appear in events, evidence metadata, or audit payloads.
- Development and production data never share identifiers by accidental database reuse.

## Initial Identifier Examples

- `host_agentroom` — dedicated Agent Room production host
- `agent_pip` — persistent Hermes participant

These examples reserve semantic clarity, not a final ID serialization format.
