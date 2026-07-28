# Progress

**Last updated:** 2026-07-27

## Current Phase

Founding release implemented and applied. Production onboarding is next.

## Completed Foundations

- [x] Refined mission from passive awareness to trustworthy supervision
- [x] Defined initial customer and coding-agent wedge
- [x] Established attention and evidence as core product concepts
- [x] Clarified command, event, projection, and telemetry boundaries
- [x] Separated agent, instance, run, session, and host identity
- [x] Defined Pip as a persistent Hermes participant
- [x] Defined `host_agentroom` and `host_ingress` production topology
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

**Status:** Promotion mechanism verified; live deployment pending

### Milestone 8: First Orchestrator

**Status:** Not started

### Milestone 9: Voice and Ambient Awareness

**Status:** Not started

### Milestone 10: Vision Pro

**Status:** Not started

## Immediate Next Work

1. Configure repository protections and private vulnerability reporting.
2. Select the real public hostname and OIDC provider/client registration.
3. Establish secure deployment access and authenticated upstream trust between
   `host_ingress` and the Ubuntu Intel production host.
4. Configure Pip's least-privilege Hermes hooks, Agent Room service token, and
   MCP tools.
5. Provision and test the encrypted off-host backup destination.

## Current Blockers

- repository protection and private vulnerability-reporting configuration
- public hostname and authentication provider
- production access and secret-management path
- authenticated `host_ingress` to production-host trust material
- encrypted backup destination and retention
- Hermes production credential and live hook/MCP configuration
