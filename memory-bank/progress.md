# Progress

**Last updated:** 2026-10-14

## Current Phase

First spatial workspace release candidate is complete locally on
`feat/spatial-workspace`. The observatory, authorized snapshots, typed Sentry
observations, accessible inspection, safe controls, and WebXR implementation
are built and locally verified. The Mac staging deployment is running over
Tailscale HTTPS with OIDC, persistent databases, supervised service operation,
and verified live updates in Chromium/WebKit. Physical Vision Pro acceptance
and remote production promotion remain separate.
[Release progress](releases/spatial-workspace.md) is tracked by Sentry.

Spatial checkpoint `95f166b` is committed. The independent continuous Sentry
publisher is implemented and deployed for the existing Agent Room project:
13 real task records, durable retry, complete scan reports, no backlog or
quarantine, and live Chromium/WebKit acceptance. Source collection, delivery
receipt, and connection health have separate labels. Full portfolio provisioning
was rejected by automatic approval review and remains a separate scope decision;
no additional projects or operator memberships were created. Read the
[continuous-feed release](releases/continuous-sentry-observations.md).

## Completed Foundations

- [x] Refined mission from passive awareness to trustworthy supervision
- [x] Defined initial customer and coding-agent wedge
- [x] Established attention and evidence as core product concepts
- [x] Clarified command, event, projection, and telemetry boundaries
- [x] Separated agent, instance, run, session, and host identity
- [x] Defined Pip as a persistent Hermes participant
- [x] Defined co-located Caddy and loopback Agent Room production topology
- [x] Defined build-once development-to-production contract
- [x] Reordered roadmap around validated supervisory outcomes

## Implementation Roadmap

### Milestone 1: Real Work Ingestion

**Status:** Implemented and verified locally

- minimal local daemon
- event store and deterministic projections
- one automatic coding-agent adapter
- source provenance
- reconnect-safe ingestion

### Milestone 2: Timeline and Evidence

**Status:** Implemented and verified locally

- browser UI
- active work and timeline
- task state
- evidence and artifacts
- replay and filtering

### Milestone 3: Attention Inbox

**Status:** Implemented and verified locally

- situation detection
- attention lifecycle
- deduplication and acknowledgement
- brief and recommendations

### Milestone 4: Safe Human Control

**Status:** Implemented and verified locally

- approvals and interventions
- policy evaluation
- budgets
- command audit

### Milestone 5: Second Runtime Adapter

**Status:** Implemented and verified locally

### Milestone 6: Hermes Adapter and Pip

**Status:** Integration implemented; live Pip configuration pending

### Milestone 7: Production on host_agentroom

**Status:** Simplified promotion mechanism verified; live deployment applying

### Milestone 8: First Orchestrator

**Status:** Not started

### Milestone 9: Voice and Ambient Awareness

**Status:** Not started

### Milestone 10: Vision Pro

**Status:** Desktop/WebXR local tailnet deployment verified; physical-device acceptance pending

The first observatory reuses the existing control plane and MiradorXR's technical
patterns. It adds desktop WebGPU/WebGL fallback and dedicated WebGL WebXR, with
shared selection, natural-input contracts, and in-world task/evidence inspection.
No headset frame-rate or comfort measurement has been claimed.

## Immediate Next Work

1. Accept the interface on the actual Vision Pro build, then choose a release
   identifier for signed promotion.
2. Resolve whether to provision the remaining 96 discovered Sentry projects;
   the current publisher is deliberately scoped to the existing project.
3. Verify the operator-designated remote machine's platform and deployment
   configuration before signed promotion; the current Mac deployment is staging
   for headset testing. The actual host identifier stays in the private runtime
   record. The July status does not prove current remote activation.
4. Configure Pip's least-privilege Hermes hooks, Agent Room service token, and
   MCP tools.
5. Provision and test the encrypted off-host backup destination.

## Current Blockers

- repository protection and private vulnerability-reporting configuration
- OIDC client registration and secret
- encrypted backup destination and retention
- Hermes production credential and live hook/MCP configuration
