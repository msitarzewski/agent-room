# Product Context

**Last updated:** 2026-07-27

## Initial Customer

A developer or technical founder concurrently supervising roughly two to ten coding agents across repositories, terminals, tools, and machines.

## Jobs to Be Done

When returning after an absence, the user needs to understand current work and blockers without reopening every terminal.

When an agent claims completion, the user needs to inspect evidence rather than trust a status message.

When work needs a decision, the user needs the issue, context, options, and safest actions in one place.

When multiple agents overlap, the user needs to detect stale, duplicate, or conflicting work before it causes damage.

When granting more autonomy, the user needs enforceable permissions, budgets, approvals, and attribution.

## Differentiation

Agent Room is not primarily a low-level LLM trace viewer.

Existing agent frameworks and observability tools already capture model calls, tool calls, handoffs, and traces. Agent Room operates above that layer:

- cross-runtime fleet and work awareness
- semantic task and organizational state
- human attention routing
- evidence-backed completion
- safe intervention and approval
- persistent participant identity
- local-first operation with open integration surfaces

Raw telemetry should use compatible OpenTelemetry conventions where practical. Agent Room Protocol adds domain semantics for commands, coordination events, evidence, attention, and control.

## Product Principles

1. Attention over activity.
2. Evidence over claims.
3. Safe intervention over passive observation.
4. Automatic capture over manual reporting.
5. Open standards over proprietary telemetry.
6. Local trust first; hosted scale later.
7. Awareness before immersion.
8. Each milestone eliminates a recurring supervisory action.

## Core Experience

The default experience answers:

1. What needs me now?
2. What is currently working?
3. What changed since I last looked?
4. What evidence supports completed work?
5. What should happen next?

The primary surface is an attention inbox supported by active work, timeline, evidence, chat, cost, and health views.

## Success Measures

- First worker connected and visible in under 10 minutes.
- Two runtimes represented without provider-specific UI.
- Active work and blockers understood within 30 seconds after a 30-minute absence.
- Every completion claim links to inspectable evidence.
- Manual terminal polling reduced by at least 80% during dogfooding.
- Fewer than one unnecessary interruption per user per workday during validation.
- Every important action records actor, policy decision, and result.
- Deployment to and rollback on `host_agentroom` is repeatable.

## Validation Risks

- Users may prefer native worker interfaces when only a few agents are active.
- Automatic task inference may be less reliable than explicit worker reporting.
- A noisy attention system may create more work than it removes.
- “Completion evidence” varies substantially across coding, research, and operational tasks.
- Chat participation may be confused with action authorization.
- A homelab production topology raises availability and public-edge security requirements early.

Validation should test these assumptions before expanding scope.
