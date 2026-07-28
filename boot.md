# Agent Room

## The Human Control Plane for AI Workers

**Version:** 0.3 (Product Foundation)

**Status:** Vision + Product and Technical Design
**Author:** Michael Sitarzewski, with GPT collaboration

---

# Executive Summary

Agent Room is the human control plane for AI workers.

It is **not** another LLM.

It is **not** another IDE.

It is **not** another chat application.

It gives people one trusted place to understand what their AI workers are doing, identify what needs attention, and safely intervene.

The initial product serves developers and technical founders supervising several coding agents across terminals, repositories, tools, and machines.

Today's AI tooling optimizes for one conversation with one model.

Humans, however, evolved to understand teams—not terminals.

We effortlessly perceive:

- who is speaking
- who is busy
- who needs help
- when something important happens
- where attention should move
- what evidence supports a claim
- what action is safe to take next

Agent Room applies those same instincts to AI systems.

Workers continue communicating through deterministic structured data and existing open standards.

Humans experience that work through visual awareness, timelines, ambient voice, and spatial presence.

The first objective is not realism or immersion.

The objective is trustworthy supervision:

1. **Awareness** — What is happening?
2. **Judgment** — What deserves attention?
3. **Control** — What can I safely do about it?

The initial product promise is simple:

> **One place to supervise every AI worker.**

---

# Vision

The first version begins in a browser.

You open Agent Room and immediately see:

- work awaiting your decision
- workers that are active, idle, blocked, or disconnected
- the evidence behind completed work
- the safest available next actions

Within seconds, you understand the state of your AI team.

As the product matures, imagine sitting down at your desk and putting on your Vision Pro.

Around you are eight AI workers.

One quietly says:

> "Builder finished authentication."

Another interrupts:

> "Reviewer rejected two commits."

Behind you:

> "Research found three properties in Portugal."

You don't reconstruct state from logs.

You don't refresh dashboards.

You know what your AI organization is doing, what needs you, and what you can do next.

That is Agent Room.

---

# Initial Customer and Job

The first customer is a developer or technical founder running approximately two to ten coding agents concurrently.

They may use Codex, Claude Code, Hermes, local agents, CI workers, or custom systems across one or more repositories.

Their recurring jobs are:

- return after an absence and reconstruct state quickly
- see which workers are active, blocked, finished, or awaiting approval
- inspect evidence behind claims of completion
- detect stale, duplicate, or conflicting work
- approve, pause, resume, redirect, review, or cancel work safely
- understand time, token, and cost consumption

The initial wedge is supervision of coding agents.

The architecture may later support research, operations, property search, and other knowledge work, but those markets must not dilute the first product.

---

# Core Philosophy

## Structured State Is Truth

Speech is presentation.

Every state-changing action is represented as a structured, attributable command.

Text and speech may propose an action. They do not silently execute it.

Voice is simply another renderer.

No agent communicates through synthesized speech.

Speech exists exclusively for humans.

---

## Humans Supervise

The system should naturally answer:

- Who is working?
- What changed?
- What is blocked?
- What deserves my attention?
- What should happen next?
- What evidence proves the claimed result?
- What decisions or approvals are waiting on me?
- What action can I take safely?

without requiring humans to dig through logs.

---

## Attention Over Activity

Raw activity is not automatically useful.

Agent Room converts information through four layers:

```
Raw Telemetry → Semantic Event → Situation → Human Attention Item
```

An attention item explains:

- why the situation matters
- what changed
- what evidence supports it
- who owns the next decision
- what actions are available
- how urgent the decision is

The product should minimize interruptions, not maximize notifications.

---

## Evidence Over Claims

A worker saying "complete" is not proof of completion.

Completion should link to inspectable evidence such as:

- diffs and commits
- tests and build results
- reviews and approvals
- generated artifacts
- source references
- unresolved risks or caveats

Agent Room preserves both the claim and the evidence used to evaluate it.

---

## Dogfood Immediately

Every milestone must eliminate a recurring supervisory action.

Examples include polling terminals, asking for status, reconstructing context, locating evidence, or manually routing a review.

The software should supervise its own construction as early as possible.

Success means Agent Room eventually becomes the primary interface used to build Agent Room.

Dogfooding is evidence, not exemption: internal use does not replace external user validation.

---

## Pluggable Everything

Replaceable components include:

- models
- voices
- queues
- storage
- frontends
- orchestrators
- adapters

The core domain remains vendor-neutral. Provider-specific behavior is isolated in adapters.

---

# Design Principle

## Agent Room is a Human Control Plane

Agent Room includes dashboards, but it is not merely a dashboard.

Agent Room is **not** an agent framework.

Agent Room does not need to own the model loop or replace the worker runtime.

It sits above worker runtimes and below the human, normalizing work state across otherwise incompatible systems.

Every connected worker reports through Agent Room.

Every authorized human supervises through Agent Room.

Agent Room becomes the canonical coordination record for normalized work state.

Git, CI systems, model providers, issue trackers, and worker runtimes remain authoritative for the facts they originate. Agent Room preserves provenance rather than pretending to replace those systems of record.

Over time, the control plane may gain operating-system-like capabilities such as scheduling, policy enforcement, resource governance, and isolation. Those capabilities must be earned rather than assumed.

---

# System Architecture

```
 Human Experience
 Browser / Attention Inbox / Timeline / Chat / Voice / Vision Pro
                              │
                              ▼
 Human Control Plane
 Approvals / Interventions / Policy / Audit / Recommendations
                              │
                              ▼
 Canonical Coordination Record
 Agents / Runs / Tasks / Events / Situations / Evidence / Artifacts
                              │
                              ▼
 Integration and Data Plane
 Adapters / ARP / OpenTelemetry / REST / WebSocket / MCP / Hooks
                              │
        ┌─────────────────────┼─────────────────────┐
        │                     │                     │
   Coding Agents           Hermes Pip             External Systems
 Codex / Claude Code   Persistent Participant   Git / CI / Trackers
```

---

# Agent Room Core

The daemon owns normalized Agent Room state for one deployment.

Responsibilities:

- Agent Registry
- Run and Session Registry
- Task Registry
- Event Store
- Situation and Attention Engine
- Evidence and Artifact Index
- Policy Engine
- Approval and Intervention Log
- Projections and Timeline
- Local API
- Authentication
- Authorization
- Audit
- Configuration

The daemon is the authority for accepted Agent Room commands, derived coordination state, and the audit trail.

Everything else is a client.

External systems remain authoritative for their native facts. Every imported fact records its source.

---

# Agent Room Protocol (ARP)

ARP is the domain contract for Agent Room commands, events, evidence, and control actions.

It is transport-independent, versioned, idempotent, and designed to coexist with established standards.

Integration surfaces include:

- CLI
- Unix Socket
- HTTP
- WebSocket
- Event or message streams
- Webhooks
- Agent lifecycle hooks

OpenTelemetry should carry compatible raw traces, metrics, and logs rather than being replaced by a proprietary telemetry format.

MCP should expose Agent Room tools, resources, and prompts to AI workers. It is an agent-facing interaction protocol, not merely another transport.

The protocol remains identical regardless of transport.

This prevents vendor lock-in.

---

# Commands vs Events

The protocol separates commands from events.

## Commands

Commands request change.

Examples:

- Register Agent
- Claim Task
- Pause Task
- Resume Task
- Spawn Worker
- Request Review
- Approve Action
- Reject Action
- Cancel Task

Commands are validated.

Accepted commands produce immutable events.

Every command includes an idempotency key, actor identity, authorization context, and expected target version when concurrency matters.

Rejected commands produce an attributable decision record without changing domain state.

---

## Events

Events report facts.

Examples:

- Task Claimed
- Tests Started
- Tests Passed
- Tests Failed
- Review Requested
- Build Completed
- Research Finished
- Human Interrupted

Events are immutable.

Events describe reality.

Deterministic reducers consume ordered events to derive current state and read-optimized projections.

This keeps replay, audit, recovery, and synchronization deterministic.

Commands express intent. Events record accepted facts. Projections answer current-state questions.

---

# Event Schema

Every event contains:

```json
{
  "version": "1.0",
  "event_id": "evt_...",
  "occurred_at": "...",
  "recorded_at": "...",
  "organization_id": "...",
  "project_id": "...",
  "agent_id": "...",
  "run_id": "...",
  "session_id": "...",
  "task_id": "...",
  "actor": {
    "type": "agent",
    "id": "..."
  },
  "source": {
    "system": "hermes",
    "instance": "hermes_primary",
    "native_id": "..."
  },
  "correlation_id": "...",
  "causation_id": "...",
  "idempotency_key": "...",
  "sequence": 42,
  "priority": "normal",
  "type": "task.progress",
  "summary": "...",
  "details": {},
  "evidence": [],
  "artifacts": [],
  "sensitivity": "internal",
  "schema": "arp://events/task.progress/1.0"
}
```

Durable domain facts become events.

High-volume raw telemetry remains in an observability stream and may be summarized into semantic events. This separation prevents the coordination record and human experience from becoming a log firehose.

Payload schemas are versioned independently. Consumers must ignore unknown additive fields and reject incompatible breaking versions explicitly.

---

# Agent Registry

Agent Room distinguishes four related identities:

- **Agent** — persistent organizational identity, such as Pip or Reviewer
- **Instance** — a running installation or process on a host
- **Run** — one bounded execution of work
- **Session** — conversational context that may span or contain runs

Every persistent agent has:

- ID
- Name
- Role
- Voice
- Avatar
- Color
- Priority
- Capabilities
- Policy Profile
- Trust Level
- Home Runtime and Host
- Presence State
- Current Runs
- Session History

Workers become persistent identities.

Instances and runs remain disposable. Their records link back to the persistent identity and preserve source-native identifiers.

Pip is the initial persistent Hermes participant. Pip may run alongside Agent
Room or on another authorized Hermes host; participant and infrastructure
identities remain separate in the data model.

---

# Task System

Every task has:

- ID
- Owner
- Status
- Dependencies
- Evidence
- Artifacts
- Priority
- Review State
- Approval State
- Budget
- Source and Native ID
- Version

Core statuses:

```
Inbox → Ready → Working → Review → Completed → Archived
           │        │         │         │
           ├────────┴──→ Blocked ───────┤
           │                            │
           └──────────→ Cancelled       └──→ Reopened → Ready

Any active state → Failed
Blocked → Ready | Working | Cancelled
Review → Working | Completed
```

The Kanban board becomes the living representation of work.

Blocked, failed, cancelled, and awaiting approval are explicit conditions with reasons, owners, and timestamps. They are not assumed to be mandatory sequential stages.

A completion claim does not become verified completion until its required evidence and review policy are satisfied.

---

# Agent Adapters

Every external system receives a lightweight adapter.

Examples:

Claude Code Adapter

Codex Adapter

Hermes Adapter

Gemini Adapter

CI Adapter

Human CLI

Adapters simply translate native behavior into ARP.

Adapters remain intentionally small.

Adapters should prefer automatic, native integration surfaces over requiring a worker to remember manual reporting steps.

An adapter may combine:

- native lifecycle hooks
- structured CLI output
- API or WebSocket streams
- OpenTelemetry export
- webhooks
- MCP tools and resources
- periodic reconciliation

Every adapter declares its supported capabilities, source-of-truth boundaries, delivery guarantees, and permission requirements.

---

## Hermes Adapter and Pip

Hermes provides several useful integration surfaces:

- lifecycle hooks for sessions, agent steps, and tool calls
- MCP client support
- a gateway with persisted sessions
- an API server for session management and chat
- webhook ingestion

The initial Hermes adapter should use hooks to emit lifecycle and tool milestones into Agent Room, then reconcile session state through the Hermes API when needed.

Agent Room should expose an MCP server to Hermes so Pip can:

- inspect workers, tasks, situations, and evidence
- claim or update authorized work
- post meaningful milestones
- request human attention or review
- attach artifacts
- participate in Agent Room chat under the persistent identity `agent_pip`

Hermes remains the authority for Pip's conversation history, memory, skills, and native sessions. Agent Room stores normalized coordination state and source links, not a competing copy of Hermes internals.

Pip must use a least-privilege policy profile. Participation in chat does not automatically grant permission to execute control-plane actions.

---

# Agent Skills

Skills teach behavior.

Skills are **not** interfaces.

Skills are policy.

Example:

When work begins:

1. Register
2. Claim task
3. Emit meaningful milestones automatically when native hooks allow
4. Attach artifacts
5. Request review
6. Mark complete

The transport is irrelevant.

Skills should teach workers how to collaborate well. They must not be the only mechanism keeping authoritative state correct; adapters and reconciliation enforce that boundary.

---

# Voice Runtime

Voice Runtime consumes events.

Responsibilities:

- Voice selection
- Speech synthesis
- Priority filtering
- Summarization
- Interruptions
- Cooldowns
- Playback

Voice Runtime never owns state.

It renders awareness.

By default, it consumes attention items and curated summaries rather than every raw event.

Voice is successful when it reduces polling without increasing interruption fatigue.

---

# Spatial Audio

Every worker occupies a location.

Example:

```
Research      Left

Builder       Front Left

Reviewer      Center

Planner       Front Right

Debugger      Right

Hermes        Behind
```

Humans begin recognizing workers by ear.

---

# Event Priorities

Low

Silent

Medium

Batched

High

Spoken

Critical

Immediate interruption

Example:

Tests Started

→ silent

Tests Passed

→ spoken

Tests Failed

→ interrupt

Repository Corrupted

→ interrupt everything

Priority alone is insufficient. Delivery policy also considers:

- whether a human decision is required
- deadline and reversibility
- confidence and evidence quality
- whether the issue is already acknowledged
- cooldowns and repeated-event suppression
- the user's current focus and notification policy

---

# Visual Interface

Think:

Mission Control

Bloomberg Terminal

NOC Wall

Not Slack.

Not Discord.

Panels:

- Attention Inbox
- Active Workers and Runs
- Tasks and Review Queue
- Timeline
- Evidence and Artifacts
- Chat
- Costs and Budgets
- Health and Connectivity
- Active Sessions
- Raw Telemetry Explorer

The default view answers five questions:

1. What needs me now?
2. What is currently working?
3. What changed since I last looked?
4. What evidence supports completed work?
5. What should happen next?

Raw logs remain available for diagnosis but are not the primary navigation model.

Chat is a shared supervisory surface. Human participants and authorized persistent agents such as Pip have explicit identities. Messages, commands, and resulting actions remain distinct records so conversational text cannot silently mutate state.

---

# Orchestration

Orchestration is **not** part of the protocol.

It is another client.

Once every worker reports through Agent Room, orchestration becomes possible.

Possible capabilities:

- Assign idle workers
- Spawn reviewers
- Merge duplicate work
- Detect blocked tasks
- Escalate failures
- Pause lower-priority work
- Recommend next actions
- Balance workloads
- Predict completion

The orchestrator is optional.

The protocol remains simple.

---

# Trust and Safety Model

Agent Room is a control plane. Security and attribution are product behavior, not deployment polish.

Every deployment must define:

- authenticated human, service, and agent identities
- role- and capability-based authorization
- explicit policy for read, write, approve, intervene, and administer actions
- least-privilege credentials for each adapter
- immutable attribution for commands, approvals, and automated actions
- secret storage outside source control and event payloads
- input validation and output encoding at every external boundary
- sensitivity labels, redaction, and retention policies
- cost, token, time, concurrency, and tool budgets
- idempotency, replay protection, and optimistic concurrency
- backup, restore, and audit-export procedures

Actions are classified by reversibility and impact:

- **Observe** — read-only; allowed by role
- **Suggest** — recommends a change; never executes it
- **Reversible Act** — may execute under an explicit policy and budget
- **Important Act** — requires approval unless a narrowly scoped policy grants it
- **Destructive or Irreversible Act** — always requires an attributable human approval

Chat participation never implies control permission.

Public ingress never implies public access.

---

# Environments and Deployment

Agent Room begins local-first and is designed for deterministic promotion from development to production.

## Development

Development runs on a developer-controlled machine with isolated configuration, credentials, ports, and data.

Development may use local adapters and test fixtures, but it must never connect to production state with write permission.

## Production

The supported production target is a dedicated Ubuntu Linux/amd64 application
host identified in examples as `host_agentroom`.

The Hermes participant named Pip connects through an independently managed
Hermes installation and least-privilege adapter credentials.

An independently managed ingress host, identified in examples as
`host_ingress`, is the public boundary. Caddy terminates public TLS and reverse
proxies only the required Agent Room routes to `host_agentroom` over an
authenticated private network.

```
Public Internet
      │
      ▼
host_ingress: Caddy + TLS + edge policy
      │ private-network upstream
      ▼
host_agentroom: Agent Room + persistent data
      ├── Hermes Agent identity: Pip
      └── Agent Room adapters and workers
```

Infrastructure and participant identity must remain distinct:

- host ID: `host_agentroom`
- persistent agent ID: `agent_pip`

## Promotion Contract

Development and production use the same versioned build artifact.

Promotion follows:

```
Source → Test → Build Once → Verify Artifact → Backup → Migrate → Deploy to host_agentroom
       → Health Check → Smoke Test → Promote Complete or Roll Back
```

Required properties:

- no production build from an uncommitted working tree
- immutable, checksummed, versioned artifacts
- environment-specific configuration injected at runtime
- secrets stored outside the repository and build artifact
- separate development and production databases
- forward-compatible migrations with a tested restore path
- health and readiness endpoints
- graceful shutdown and restart
- a last-known-good artifact and documented rollback command
- automated post-deploy smoke checks through both private and public paths
- database and artifact backups before irreversible migrations

## Network Boundary

Only Caddy on `host_ingress` is directly exposed to the public internet.

The Agent Room service on `host_agentroom` should bind to a private interface or otherwise restrict inbound traffic to the trusted network and `host_ingress`.

Caddy must:

- terminate HTTPS
- proxy WebSocket connections where required
- enforce request size and timeout limits
- preserve and validate trusted proxy information
- apply authentication before protected routes when authentication is delegated at the edge
- expose only intentional public routes
- perform upstream health checks
- avoid disabling upstream TLS verification when TLS is used internally

Administrative, debugging, raw telemetry, and unauthenticated health-detail routes remain private.

Deployment automation must not modify the independently managed Hermes installation or its persisted state. Agent Room and Hermes have separate lifecycle, data, backup, and rollback boundaries even when colocated on `host_agentroom`.

---

# Development Roadmap

---

## Milestone 1

### Real Work Ingestion

Features

- minimal local `agentroomd`
- event store and deterministic projections
- REST and WebSocket APIs
- one automatic coding-agent adapter
- agent, instance, run, session, and task registration
- source provenance and reconnect-safe ingestion

Dogfood

Observe real Agent Room development without requiring manual status narration.

Exit evidence

- connect one worker in under ten minutes
- recover from duplicate and out-of-order delivery
- trace every normalized fact to its native source

---

## Milestone 2

### Timeline and Evidence

Features

- live browser interface
- active workers and runs
- task state
- streaming timeline
- evidence and artifact links
- filtering and replay
- reconnect and catch-up

Dogfood

Return after an absence and reconstruct project state without polling terminals.

Exit evidence

- identify active work, recent changes, and blockers within thirty seconds
- every completion claim links to inspectable evidence

---

## Milestone 3

### Attention Inbox

Features

- situation detection
- deduplication and acknowledgement
- attention ownership and deadlines
- recommended next actions
- morning and return-from-away brief
- configurable interruption policy

Dogfood

Let Agent Room identify what deserves attention during its own construction.

Exit evidence

- critical situations are surfaced reliably
- acknowledged issues do not repeatedly interrupt
- users receive fewer than one unnecessary interruption per workday during validation

---

## Milestone 4

### Safe Human Control

Features

- approve and reject
- pause, resume, message, redirect, and cancel
- policy evaluation
- budgets
- immutable command and approval audit trail
- optimistic concurrency and idempotency

Dogfood

Manage real work from Agent Room rather than switching back to each worker interface.

Exit evidence

- every control action has an attributable actor and policy decision
- stale or duplicate commands cannot execute twice
- destructive actions cannot bypass human approval

---

## Milestone 5

### Second Runtime Adapter

Connect a second vendor or runtime without changing the core domain or primary interface.

Candidate order should follow actual daily usage, not vendor preference:

- Codex
- Claude Code
- CI
- custom local worker

Dogfood

Supervise two different worker runtimes in the same project view.

Exit evidence

- the second adapter requires no provider-specific UI
- normalized behavior remains attributable to its native source

---

## Milestone 6

### Hermes Adapter and Pip

Features

- Hermes lifecycle hook integration
- Hermes session reconciliation
- Agent Room MCP server
- persistent identity `agent_pip`
- Pip participation in shared chat
- least-privilege task and attention tools
- independent Hermes and Agent Room lifecycle boundaries

Dogfood

Pip participates as a visible teammate while Agent Room is built and deployed on the `host_agentroom` host.

Exit evidence

- Pip can inspect authorized coordination state and attach evidence
- Pip can request attention without receiving implicit administrative control
- Hermes upgrades do not require Agent Room upgrades, and vice versa

---

## Milestone 7

### Production on host_agentroom

Features

- immutable build artifact
- development-to-production promotion
- Caddy ingress through `host_ingress`
- authentication and authorization
- health, readiness, and smoke checks
- migrations, backup, restore, and rollback
- production audit and resource monitoring

Dogfood

Use the production installation as the daily control room for Agent Room development.

Exit evidence

- deployment and rollback are repeatable
- only `host_ingress` is publicly exposed
- private and public health checks pass
- production secrets and state remain isolated from development

---

## Milestone 8

### First Orchestrator

Capabilities:

- Idle detection
- Automatic review assignment
- Block detection
- Task routing

Dogfood

Agent Room begins coordinating its own development.

Exit evidence

- every automated action is explainable and policy-authorized
- suggestions can run in shadow mode before execution is enabled

---

## Milestone 9

### Voice and Ambient Awareness

Attention items and curated summaries become spoken updates.

Capabilities:

- voice selection
- batching and cooldowns
- focus-aware interruption
- spatially recognizable workers
- replay and mute controls

Dogfood

Reduce terminal polling without increasing interruption fatigue.

---

## Milestone 10

### Vision Pro

The proven control-room experience becomes immersive.

Floating workspaces.

Spatial audio.

Eye tracking.

Gesture interaction.

Vision Pro is an experience layer over the same control plane. It must not require a separate source of truth or provider-specific behavior.

---

# Long-Term Vision

Eventually:

You no longer open terminals.

You no longer watch CI dashboards.

You no longer monitor logs.

You open Agent Room.

You ask:

> "Good morning."

It replies:

> "Builder finished the authentication refactor."

> "Reviewer rejected two pull requests."

> "Hermes found five new candidate properties."

> "Research is waiting on legal clarification."

> "Planner recommends assigning Builder-2 to API work."

Within ten seconds, you understand the state of your AI organization.

Not because you searched for information.

Because the room told you what mattered, showed you the evidence, and offered the right controls.

---

# Guiding Analogy

> **Agent Room is the human control plane for autonomous work.**

Kubernetes remains a useful architectural analogy:

- it did not invent containers
- it standardized how they are observed and managed
- it made state, policy, and control explicit

But Agent Room must not borrow the analogy's promises before it provides equivalent scheduling, isolation, resource governance, and reliability.

The immediate product standardizes how AI workers report work, present evidence, request attention, and accept authorized human control.

---

# Design Goals

- Transport agnostic
- Vendor agnostic
- Deterministic
- Observable
- Extensible
- Dogfood from day one
- Human-first supervision
- Attention over activity
- Evidence over claims
- Safe intervention
- Local-first trust
- Open-standard compatibility
- AI-native architecture
- Designed first for two to ten workers, with a path to larger teams

---

# Success Criteria

Initial product success is achieved when:

- a developer connects the first worker and sees real activity in under ten minutes
- two different worker runtimes appear in one coherent interface
- after thirty minutes away, the developer understands active work and blockers within thirty seconds
- every completion claim links to inspectable evidence
- every important control action has an attributable actor, policy decision, and audit record
- manual terminal polling falls by at least eighty percent during dogfooding
- critical attention items are acknowledged or escalated
- unnecessary interruptions average fewer than one per user per workday during validation
- production can be deployed to and rolled back on `host_agentroom` repeatably
- public access through `host_ingress` never exposes an unauthenticated control or administrative surface

Long-term success is achieved when a person can supervise an organization of AI workers without reconstructing state from logs, switching among worker applications, or repeatedly polling status.

They enter the room, understand what matters, inspect the evidence, and act safely.
