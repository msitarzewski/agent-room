# Quick Start

**Last updated:** 2026-07-27

## Project in One Paragraph

Agent Room is the human control plane for AI workers. It begins as a local-first browser experience for developers supervising two to ten coding agents. It normalizes work across runtimes, routes human attention, connects completion claims to evidence, and enables safe, attributable intervention.

## Read Order

1. `boot.md` — founding product and technical specification
2. `memory-bank/activeContext.md` — current task and decisions
3. `memory-bank/projectbrief.md` — stable scope
4. `memory-bank/systemPatterns.md` — architecture
5. `memory-bank/techContext.md` — known environment and open choices
6. `memory-bank/progress.md` — roadmap state

## Deployment Roles

- `host_agentroom`: dedicated Ubuntu Linux/amd64 Agent Room host
- Pip / `agent_pip`: persistent Hermes participant connected through the Hermes adapter
- `host_ingress`: independently managed Caddy public ingress

Do not confuse `host_agentroom` with `agent_pip`.

## Foundational Boundaries

- Agent Room owns normalized coordination state, attention, approvals, and audit.
- External systems retain authority for their native facts.
- Hermes retains Pip's native memory, skills, conversations, and sessions.
- Chat messages do not directly mutate state.
- Destructive actions require human approval.
- Only `host_ingress` is intentionally exposed to the public internet.
- Development and production use separate data and credentials.
- One verified artifact is promoted to `host_agentroom`.

## Current Status

The founding control-plane release is implemented and verified locally.
Production onboarding and the live Hermes/Pip connector remain pending.

## Next Decision

Select the concrete production hostname, OIDC provider, secure deployment
transport, backup destination, and live Hermes/Pip credentials without placing
environment secrets in source.
