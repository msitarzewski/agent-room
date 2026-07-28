# 270726 Product Foundation and Memory Bank

## Objective

Refine the Agent Room founding specification into a focused product and
technical foundation, establish the initial Memory Bank, and define the
development-to-production boundary for a dedicated `host_agentroom` behind
Caddy on `host_ingress`.

## Outcome

- ✅ Mission refined to trustworthy supervision: awareness, judgment, and safe control
- ✅ Initial customer defined as a developer or technical founder supervising two to ten coding agents
- ✅ Attention, evidence, identity, authority, and safe intervention established as domain concepts
- ✅ Hermes participant Pip defined separately from the `host_agentroom` host
- ✅ Hermes integration constrained to documented hooks, API, MCP, sessions, and authenticated webhooks
- ✅ Development-to-production promotion, health, backup, migration, smoke, and rollback contracts defined
- ✅ Roadmap reordered around real ingestion, evidence, attention, and control before voice and spatial interfaces
- ✅ Foundational Memory Bank created and indexed
- ✅ Documentation QA passed
- ✅ Review and application explicitly approved

## Files Modified

- `boot.md` — product mission, customer, architecture, protocol, Hermes/Pip integration, trust model, deployment topology, roadmap, and measurable success criteria
- `memory-bank/toc.md` — Memory Bank index
- `memory-bank/projectbrief.md` — stable mission and scope
- `memory-bank/productContext.md` — customer, jobs, differentiation, and measures
- `memory-bank/systemPatterns.md` — architecture and integration patterns
- `memory-bank/techContext.md` — environment, documented integration surfaces, and open stack choices
- `memory-bank/database-schema.md` — conceptual entities and invariants
- `memory-bank/build-deployment.md` — `dev → host_agentroom` deployment and `host_ingress` ingress contract
- `memory-bank/testing-patterns.md` — quality, security, adapter, migration, and deployment testing
- `memory-bank/projectRules.md` — product, architecture, safety, and deployment rules
- `memory-bank/decisions.md` — accepted founding decisions
- `memory-bank/quick-start.md` — minimum future-session context
- `memory-bank/progress.md` — milestone and next-work state
- `memory-bank/activeContext.md` — state-machine and pending-decision context
- `memory-bank/tasks/2026-07/README.md` — monthly work index

## Patterns Applied

- `memory-bank/systemPatterns.md#Layered-Control-Plane-Architecture`
- `memory-bank/systemPatterns.md#Authority-and-Provenance`
- `memory-bank/systemPatterns.md#Telemetry-to-Attention`
- `memory-bank/systemPatterns.md#Persistent-Identity`
- `memory-bank/systemPatterns.md#Hermes-and-Pip-Pattern`
- `memory-bank/systemPatterns.md#Deployment-Boundary`
- `memory-bank/systemPatterns.md#Chat-Is-Not-a-Command-Bus`

## Integration Points

- Hermes lifecycle hooks provide automatic session, step, and tool milestones.
- Hermes API supports session reconciliation and chat integration where appropriate.
- Agent Room MCP exposes authorized tools and resources to `agent_pip`.
- Caddy on `host_ingress` is the only intentional public ingress.
- Agent Room and Hermes run with separate process, data, credential, backup, upgrade, and rollback boundaries on `host_agentroom`.

## Architectural Decisions

- Agent Room is the human control plane for AI workers.
- Coding-agent supervision is the initial product wedge.
- Attention and evidence are core domain concepts.
- External systems retain authority for native facts.
- `host_agentroom` and `agent_pip` are distinct typed identities.
- Hermes integrates only through documented extension surfaces.
- Production runs on `host_agentroom` behind Caddy on `host_ingress`.
- One immutable verified artifact is built and promoted.

See `memory-bank/decisions.md` for rationale and consequences.

## QA

- Required file presence: PASS
- Memory Bank relative links: PASS
- Event-schema JSON parsing: PASS
- Markdown code-fence balance: PASS
- Diff whitespace check: PASS
- Contradictory legacy-language scan: PASS
- Design-level security review: PASS
- Application tests, build, and coverage: not applicable; implementation has not started

## Artifacts

- Specification: `boot.md`
- Memory Bank: `memory-bank/toc.md`
- Deployment contract: `memory-bank/build-deployment.md`
- Version-control commit or PR: none; the workspace is not yet a Git repository
