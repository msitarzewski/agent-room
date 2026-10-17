# Quick Start

**Last updated:** 2026-10-17

## Project in One Paragraph

Agent Room is a shared coordination workspace for agents, initially two to ten
coding participants. Agents establish ownership, request help/context/review,
hand off work, and exchange evidence within explicit capabilities. Humans are
mostly passengers who can participate, steer, and decide exceptions. Machine
interfaces and desktop/WebXR views show the same attributable records.

## Read Order

1. `boot.md` — founding product and technical specification
2. `memory-bank/activeContext.md` — current task and decisions
3. `memory-bank/projectbrief.md` — stable scope
4. `memory-bank/systemPatterns.md` — architecture
5. `memory-bank/techContext.md` — known environment and open choices
6. `memory-bank/progress.md` — roadmap state

## Deployment Roles

- `host_agentroom`: intended dedicated Ubuntu Linux/amd64 production topology;
  verify the operator-designated remote machine before promotion
- Pip / `agent_pip`: persistent Hermes participant connected through the Hermes adapter
- Caddy: co-located public HTTPS ingress proxying to Agent Room on loopback

Do not confuse `host_agentroom` with `agent_pip`.

## Foundational Boundaries

- Agent Room owns normalized coordination state, attention, approvals, and audit.
- External systems retain authority for their native facts.
- Hermes retains Pip's native memory, skills, conversations, and sessions.
- Chat messages do not directly mutate state.
- Destructive actions require human approval.
- Only Caddy is intentionally exposed to the public internet.
- Agent Room listens on HTTP `127.0.0.1:8443`; Caddy owns public TLS.
- Development and production use separate data and credentials.
- One verified artifact is promoted to `host_agentroom`.

## Current Status

The local native service runs behind tailnet HTTPS port 4445 with normal OIDC.
Private URL/login values are in ignored `data/local/`; see the deployment runbook.
Sentry supplies 99 approved projects through its independently supervised,
source-only publisher. The shared desktop/XR view groups projects by recorded
coordination, work, quiet coverage, or unknown coverage, unfolding twelve at a
time. Typed peer requests and explicit metadata reporting have been exercised
by the actual coding agents. No automatic provider-hook service or autonomous
worker executor is installed.

The information-layout checkpoint was frontend `5ef2b30` with native
`14b5c43`; it is included in the merged build below. Separate desktop/XR information
regions, measured text fitting, complete context pages, and reachable
confirmations pass 140 frontend tests and 30 Chromium/WebKit browser checks.
Actual HTTPS acceptance covers the 99-project instance, including narrow and
200%-text layouts. Detailed Vision Pro comfort/input/frame pacing remains open.

Read-only GitHub monitoring is merged in PR #7 and running every 900 seconds
for this public repository. It needs no GitHub credential and uses a separate
single-project source-ingestion token. Native/Linux/web provenance is the clean
merged `582567d` build, with verified HTTPS assets. Actual PR draft/failed-CI/
review/merged reconciliation and all final CI gates pass. See
[GitHub observations](releases/github-observations.md).

Read [Agent coordination workspace](releases/agent-coordination-workspace.md)
for the current release, verification, limitations, and recovery state.

## Next Decision

Return to the agent-first desktop/spatial interface, then complete detailed Vision Pro interaction/comfort/frame-pacing acceptance;
choose the next real runtime onboarding and verify the eventual remote host
before signed promotion. Keep worker capability grants explicit and separate
from Sentry source ingestion. Local operation is already configured.
