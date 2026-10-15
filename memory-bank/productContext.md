# Product Context

**Last updated:** 2026-10-15

## Initial Customer

Coding agents coordinating across repositories, terminals, tools, and machines,
initially in groups of roughly two to ten. A developer or technical founder is
the beneficiary and occasional participant, mostly observing their progress.

Michael's 2026-10-15 clarification: agents are the primary users; he is "a
passenger here. mostly." The human should not become the routine work dispatcher.

## Jobs to Be Done

An agent needs to discover current context, ownership, dependencies, and evidence
without reconstructing another agent's work from unrelated tools.

When an agent needs help, review, context, or a handoff, it needs an attributable
request addressed to the responsible participant with a clear next action.

Agents should resolve routine coordination within granted capabilities and
escalate permission, policy, or unresolved decisions to the appropriate human.

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

1. Agent coordination and progress before human interruption.
2. Evidence over claims.
3. Safe intervention over passive observation.
4. Automatic capture over manual reporting.
5. Open standards over proprietary telemetry.
6. Local trust first; hosted scale later.
7. Awareness before immersion.
8. Each milestone removes coordination friction or a recurring human intervention.

## Core Experience

The default experience answers:

1. Who is working on what, and with which authority?
2. What is waiting on another agent, context, review, or a dependency?
3. What changed, and who needs to know?
4. What evidence supports a handoff or completion?
5. What can agents do next, and which exceptions need a human?

Machine-readable coordination records and authorized tools are the primary
agent interface. Browser and immersive views should make that same coordination
understandable to a passenger or participant. The current application home is
the existing attention inbox; the agent-first observatory now provides typed
coordination inspection and grouped desktop/XR context. Future automatic runtime
onboarding and autonomous orchestration remain outside the verified explicit
reporter and request workflow.

The spatial observatory adds an original portfolio → project → task → evidence
experience at `/workspace`, alongside those views. Stable geography and real
dependencies help the user regain context; persistent attention and accessible
inspectors keep decisions and evidence close. Desktop and immersive WebXR share
the same authorized records and selected work. Decorative sculpture never
implies unobserved activity, and source completion remains separate from human
acceptance. The default home continues to be Attention during device acceptance.

Michael authorized this first visual release on 2026-10-13. MiradorXR supplies
proven technical patterns; its visual design is not the template. See
`memory-bank/releases/spatial-workspace.md` for scope and validation evidence.

## Success Measures

- Agents discover ownership/context and exchange attributable requests and handoffs.
- Routine coordination reaches the responsible agent without requiring human relay.
- Human interruptions are limited to meaningful exceptions within the policy model.
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
