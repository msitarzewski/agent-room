# System Patterns

**Status:** Founding control plane and local spatial candidate validated

**Last updated:** 2026-10-17

## Layered Control-Plane Architecture

Agent Room separates five responsibilities:

1. **Integration and data plane** — receives native worker and system signals.
2. **Canonical coordination record** — stores normalized agents, work, events, situations, evidence, and audit state.
3. **Authorized coordination** — routes explicit requests, ownership, review, and policy-bound interventions between participants.
4. **Human experience** — exposes agent work and coordination through browser/spatial observation, participation, and explicit escalation.
5. **Optional orchestration** — recommends or performs policy-authorized coordination.

Dependencies point inward toward domain contracts. Presentation and adapters do not own canonical state.

## Typed Participant Requests

`internal/app/service.go:CreateCoordination()` and `ActCoordination()` extend
the existing command/event/projection/audit transaction. `coordination_request`
is a resource kind, with help/context/handoff/review intent, authenticated owner,
named recipient, same-project task/evidence links, and an explicit lifecycle.
Recipient responses and owner resolution/cancellation require capabilities,
the current version, and an idempotency key. Operator overrides retain actual
actor attribution; a human-addressed request requires human participation.

Recipient privacy applies at every read boundary: direct/list resources,
workspace snapshots, overview items, recorded events, and live stream frames.
Full project readers can observe all requests. Narrow readers see only their
owned/incoming records. Source ingestion cannot create these commands. Request
resolution never implies task acceptance, execution, or ownership transfer.
Coordination audit reads omit duplicated request/response bodies, including
historical rows; stored history remains immutable. Audit attribution stays
available and authorized participants read bodies through the request boundary.

## Honest Coordination Geography

`internal/postgres/repository.go:summarizeCoordination()` derives actionable
counts and at most 24 attributed items from one authorized project snapshot.
Supervised unfinished work, source-declared work, running runs, leased presence,
requests, reviews, and human escalations remain distinct signals. Unclaimed
source inbox cards are context; source completion is never accepted completion.
Missing read permissions make summary coverage incomplete.

`web/src/workspace/data.ts:projectRegion()` and `web/src/workspace/scene.ts:buildWorkspaceScene()`
reuse those summaries for four stable regions: Coordination, Work recorded,
Quiet, and Coverage unknown. Progressive pages reveal twelve named projects.
Known asks/work remain visible with coverage qualifiers. Quiet requires complete
authorized coverage and a complete source collection within fifteen minutes.
Labels, shapes, and text explain colors; real Agent records establish
participants. A repository never becomes an agent because it is active in Git.

## Explicit Runtime Reporting

`cmd/agentroomctl/main.go:report()` runs before daemon/database configuration.
`internal/adapters/reporter.go:Report()` persists bounded metadata observations
before private HTTP delivery. Actor/project/runtime/instance/endpoint identity
binds the private state; uncertain retries preserve exact bodies and sequences.
Only HTTP 202 acknowledges delivery; invalid/expired records remain quarantined.
HTTPS or loopback HTTP, refused redirects/proxies, owner-only files, and fixed
nonsecret operational errors protect credentials.

Typed agent/session/run observations bind `agent_id` to the authenticated actor.
Canonical session/run IDs derive from project/runtime/actor/native identity.
Presence has a five-minute lease; native runs require explicit real IDs.
Codex's explicit reporter uses `CODEX_THREAD_ID`, not its parent conversation
identifier. No transcript, raw tool body, arbitrary usage/error payload, or
conversation file is collected. This fallback observes actual callers; it does
not install provider hooks, start workers, or schedule autonomous work.
Ordinary `source:codex`, `source:claude`, and `source:hermes` credentials accept
only these three typed observations. Legacy lifecycle ingestion requires an
explicit privileged compatibility grant; it is not part of runtime reporter
permissions. Legacy codec exports need semantic mapping before live wiring.

## Authority and Provenance

Agent Room is authoritative for:

- accepted Agent Room commands
- normalized coordination state
- attention and approval state
- the Agent Room audit trail

External systems remain authoritative for their native facts:

- Git for repository history
- CI for native build and check records
- Hermes for Pip's native sessions, memory, skills, and conversation history
- each worker runtime for its native execution identifiers and raw telemetry

Imported records retain source system, instance, native ID, observation time, and correlation identifiers.

## Command, Event, Projection

- A command expresses intent and is authenticated, authorized, validated, and idempotent.
- An accepted command emits one or more immutable events.
- A rejected command emits or records an attributable decision without changing domain state.
- Ordered events drive deterministic reducers.
- Reducers produce current-state projections and read models.
- Replaying the same event stream must produce the same normalized state.

Events do not directly contain secrets or unbounded raw transcripts.

## Telemetry to Attention

Information flows through:

```
Raw Telemetry → Semantic Event → Situation → Human Attention Item
```

- Raw telemetry supports diagnosis and observability.
- Semantic events record durable domain facts.
- Situations correlate facts into meaningful conditions.
- Attention items explain why a person should care and what they can do.

Deduplication, acknowledgement, cooldown, ownership, deadline, confidence, reversibility, and focus policy determine delivery.

## Persistent Identity

Do not overload “agent.”

- `Agent` is the persistent organizational identity.
- `AgentInstance` is a concrete process or installation.
- `Run` is bounded work execution.
- `Session` is conversational context.
- `Host` is infrastructure.

For Pip:

- host: `host_agentroom`
- persistent participant: `agent_pip`
- runtime: Hermes Agent

Shared display names are allowed; canonical IDs and entity types must remain distinct.

## Evidence-Backed Completion

Completion has separate claims and verification:

- worker submits completion claim
- evidence is linked by immutable source reference where possible
- required checks are evaluated
- review policy determines whether completion is verified
- verified completion remains traceable to its claim, evidence, reviewer, and policy

Evidence includes diffs, commits, test results, build results, reviews, source citations, generated artifacts, and declared caveats.

## Adapter Pattern

Adapters translate native runtime behavior into ARP without embedding provider behavior in the core domain.

Prefer, in order:

1. native lifecycle hooks or structured event export
2. OpenTelemetry-compatible telemetry
3. supported API or WebSocket integration
4. webhook integration
5. structured CLI output
6. periodic reconciliation
7. explicit worker reporting as a fallback

Each adapter declares:

- capabilities and permissions
- native authority boundaries
- delivery guarantees
- retry and idempotency behavior
- reconciliation behavior
- supported protocol versions

Provider-specific data remains in an extension envelope and never becomes a required UI dependency.

## Hermes and Pip Pattern

The initial Hermes adapter uses:

- lifecycle hooks for session, step, and tool milestones
- the Hermes API for session reconciliation and chat when appropriate
- Agent Room's MCP server for Pip's authorized tools and resources
- webhook support only where an inbound trigger is the correct direction

Hermes and Agent Room have independent processes, persistent data, upgrades, health checks, and rollbacks.

Pip's chat permissions are separate from Pip's control permissions.

## Deployment Boundary

Production topology:

```
Internet → host_agentroom/Caddy → HTTP 127.0.0.1:8443 → Agent Room
```

Only Caddy accepts public traffic.

Agent Room's application, administration, and adapter listeners bind to
loopback. Caddy denies private adapter and MCP routes before proxying remaining
requests. Administrative and debug surfaces remain private.

The same immutable artifact is promoted from development to production with environment-specific configuration injected at runtime.

## Safe Control Classification

Every command belongs to an impact class:

- Observe
- Suggest
- Reversible Act
- Important Act
- Destructive or Irreversible Act

Policy may allow narrowly scoped reversible actions. Destructive or irreversible actions always require attributable human approval.

## Chat Is Not a Command Bus

Chat messages, command proposals, accepted commands, and resulting events are separate records.

Natural-language text may propose an action. It cannot silently mutate canonical state.

## Spatial Read Boundary

`internal/postgres/repository.go:578` reads workspace resources, the event
watermark, and source receipt times in one read-only repeatable-read transaction.
The application checks every included resource kind before returning a snapshot;
partial hidden graphs fail closed. The scene and HTML inspector share this
snapshot. GPU selection and placement never write domain state.

`web/src/hooks/useCollection.ts:56` serializes refreshes, coalesces events during
an active request, cancels project switches, and clears protected state on
authorization loss. Failed refreshes retain explicitly stale data. Stream events
are project-scoped, including reconnect and resync signals. Event cursors are
not treated as commit-order guarantees; every relevant batch refreshes state.

## Source Observations and Supervision

Sentry's typed `task.observed` contract derives stable project-scoped identity
from `external_id`. New tasks enter `inbox`; later observations update source
metadata separately from human-owned title, status, assignment, dependencies,
review, and approval. See `internal/app/service.go:608` and
`internal/postgres/projector.go:147`.

Projection identity locks serialize first delivery; source sequences reject
stale updates. Trusted command replay preserves source watermarks. Private
ingestion cannot supply resource replacements or trusted command attribution.
Source-owned blocked attention has a separate identity, resolves only from a
newer nonblocked observation, and honors human resolution until a new blocked
episode. Source completion never accepts evidence or a completion claim.

`project.observed` updates a separate source report and never changes the
operator's project name or access. Collection time, receipt time, and last
complete collection are distinct in overview/workspace responses; delayed or
out-of-order delivery cannot claim a fresh successful scan. See
`internal/app/service.go:668`, `internal/postgres/projector.go:158`, and
`internal/postgres/repository.go:637`.

The independent collector at `adapters/sentry/agentroom_sentry.py:272` reuses
Sentry's read-only scanner, persists immutable event bodies and watermarks in a
SQLite transaction before delivery, and retries with stable identity. Positive
observations only: missing, window-expired, or unreadable source records never
delete tasks. Project/path bindings and source instance persist with the queue.
Operator setup uses `internal/postgres/admin.go:63`; the publisher receives only
exact project scopes and source ingestion capabilities. Its private credentials
and queue are independent of Sentry's board, watcher, history, and backups.

## Desktop and Immersive Resource Ownership

`web/src/workspace/SceneCanvas.tsx:576` owns a desktop WebGPU renderer with WebGL2
fallback and a separate WebGL XR stage sharing domain state. The installed
Fiber/XR versions require the WebGL XR manager interface. Request immersive
sessions synchronously within user activation; handle cancellation and clean
session/store/listener/texture ownership. Move the workspace, never the headset
camera. Keep HTML directory and controls available when graphics fail.

Project identity determines stable geography. Actual dependency edges determine
task layout. Instancing, bounded labels, demand rendering, visibility pause,
and reduced motion limit scene work. The pinned production build removes the
upstream opt-in emulator branch and fails if its reviewed shape changes.

Desktop information uses normal flow: connection details and the scene toolbar,
notice, canvas and hint reserve their own rows in `web/src/styles.css:41`.
Immersive fitting reuses label geometry in `web/src/workspace/scene.ts:48` and
measures text ink in `web/src/workspace/SceneCanvas.tsx:62`. Keep the central
diagram, side information and control bands disjoint; preserve long recorded
context through pages instead of reducing it to an inaccessible clipped label.

## Read-only Repository Observation Sources

The typed task/project source allowlist is explicit at
`internal/domain/models.go:223`. Source-specific task/report IDs preserve legacy
Sentry identities and isolate GitHub at `internal/domain/models.go:227` and
`internal/domain/models.go:237`; freshness joins the exact source report at
`internal/postgres/repository.go:652`. Sources never accept canonical completion,
change ownership or create coordination commands.

`adapters/source.py:82` and `adapters/source.py:177` own shared durable state and
immutable delivery. The provider-specific API guard/cache and collector stay in
`adapters/github/agentroom_github.py:108` and
`adapters/github/agentroom_github.py:258`. Repository binding, bounded requests,
conditional caching, rate backoff and reconciliation are independent from the
Sentry scanner. Missing/partial facts retain prior context with qualifiers;
review/check facts refer to their head/attempt and never imply merge authority.
Treat source bodies as attributed data. Keep credentials and queues private.
