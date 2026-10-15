# Decisions

**Last updated:** 2026-10-14

Only accepted decisions belong here. Open technical choices remain in `techContext.md` and `build-deployment.md`.

## 2026-10-14: Local Tailnet Deployment Uses MiradorXR's HTTPS Pattern

**Status:** Local staging step toward the operator-designated remote machine.

**Context:** The first desktop/WebXR candidate needs a secure origin reachable
from Vision Pro on the existing tailnet, using this Apple Silicon workstation.

**Decision:** Run the existing native Go daemon and production web build behind
Tailscale Serve on dedicated HTTPS app port 4445. Use HTTPS port 4446 for a local
Dex identity provider, persistent isolated PostgreSQL databases, normal OIDC
authorization, and a per-user launch agent. Preserve existing Serve mappings.
Private runtime values and generated operator credentials stay in ignored
`data/local/`. This does not replace the signed Linux/Caddy promotion contract.
Michael confirmed that the remote machine is the eventual home; the local
installation supports headset testing first. Its actual host identifier is in
the private deployment record. Verify its platform before building the promoted
artifact; this decision does not authorize remote activation.

**Consequences:** The headset can use the same web application over an HTTPS
origin without a native visionOS package. The workstation, Docker Desktop, and
Tailscale must remain available. Device acceptance and production promotion
still require their own evidence. No automatic Sentry connector is installed.

**References:** `build-deployment.md#local-tailnet-deployment`;
`releases/spatial-workspace.md`.

## 2026-10-13: Shared Spatial Workspace and MiradorXR Technical Reference

**Status:** Accepted for the user-authorized first release.

**Context:** Michael requested an original magical desktop/WebXR workspace and identified the team's tested MiradorXR implementation as the technical reference.

**Decision:** Extend the existing React/TypeScript client and Go/PostgreSQL control plane. Use Three.js, React Three Fiber, and react-three/xr with shared selection and domain data. Use WebGPU on desktop where available and the proven WebGL immersive stage because installed Fiber/XR session code assumes WebGL manager interfaces. Keep all controls on the existing command API. Sentry observations retain their own source state.

**Consequences:** Input, lifecycle, loading, and resource-ownership patterns are reusable. Scene design is original. Desktop graphics and XR rendering have explicit lifecycle boundaries; no WebGPU XR capability is claimed until its actual library combination is validated.

**References:** `releases/spatial-workspace.md`; `tasks/2026-10/zoomable-control-room-plan.md`.

## 2026-07-27: Human Control Plane Mission

**Status:** Accepted

**Context:** The founding specification emphasized awareness and described Agent Room as an operating system. The initial product does not yet own scheduling, isolation, execution, or resource governance.

**Decision:** Position Agent Room as the human control plane for AI workers. The mission combines awareness, judgment, and safe control.

**Consequences:**

- the product must support intervention and approval, not observation alone
- “operating system” becomes a possible long-term architecture rather than a present capability claim
- safety and attribution become core product requirements

**Reference:** `boot.md#Executive-Summary`

## 2026-07-27: Coding-Agent Supervision Is the Initial Wedge

**Status:** Accepted

**Context:** The vision covers coding, research, operations, and large organizations, but an initial market and workflow are required.

**Decision:** Build first for a developer or technical founder supervising approximately two to ten coding agents.

**Consequences:**

- adapters, evidence, task state, and controls prioritize software-development workflows
- broader knowledge-work scenarios remain future extensions
- success is measured on small-team supervisory outcomes before scale claims

**Reference:** `boot.md#Initial-Customer-and-Job`

## 2026-07-27: Attention and Evidence Are Core Domain Concepts

**Status:** Accepted

**Context:** An event feed alone moves log-reading into a new interface and completion claims are not trustworthy without proof.

**Decision:** Model telemetry, semantic events, situations, and attention items separately. Model completion claims and supporting evidence separately from verified completion.

**Consequences:**

- the home experience centers on decisions, not event volume
- reducers and policies must support acknowledgement, ownership, deduplication, and verification
- evidence provenance becomes part of the data model

**Reference:** `boot.md#Attention-Over-Activity`, `boot.md#Evidence-Over-Claims`

## 2026-07-27: Preserve External Authority

**Status:** Accepted

**Context:** Agent Room cannot truthfully replace Git, CI, issue trackers, worker runtimes, or Hermes session storage as the authority for their native facts.

**Decision:** Agent Room is the canonical coordination record. Imported facts retain provenance and external systems remain authoritative for native data.

**Consequences:**

- adapters reconcile instead of copying ownership
- source and native identifiers are required
- Agent Room projections may be rebuilt without rewriting external systems

**Reference:** `memory-bank/systemPatterns.md#Authority-and-Provenance`

## 2026-07-27: Pip Host and Pip Participant Are Distinct Identities

**Status:** Accepted

**Context:** Infrastructure and the persistent Hermes participant require
separate identities even when they are colocated.

**Decision:** Represent them as separate typed entities: `host_agentroom` and `agent_pip`.

**Consequences:**

- infrastructure permissions cannot be confused with participant permissions
- logs, policies, sessions, and deployments remain attributable
- the UI may display the familiar shared name while internal IDs remain unambiguous

**Reference:** `memory-bank/database-schema.md#Initial-Identifier-Examples`

## 2026-07-27: Hermes Integrates Through Documented Surfaces

**Status:** Accepted

**Context:** Hermes provides lifecycle hooks, MCP support, persisted sessions, an API server, a gateway, and webhooks.

**Decision:** Build the Hermes adapter from documented hooks, APIs, MCP, and authenticated webhooks. Do not patch Hermes internals or duplicate its native memory and session authority.

**Consequences:**

- Pip can participate in chat and coordination with explicit permissions
- Hermes and Agent Room upgrade and roll back independently
- the adapter needs reconciliation and version-compatibility tests

**Reference:** `memory-bank/systemPatterns.md#Hermes-and-Pip-Pattern`

## 2026-07-27: Production Runs on host_agentroom Behind Caddy on host_ingress

**Status:** Superseded on 2026-07-28

**Context:** Agent Room needs a production topology with a single intentional
public ingress and a private application host.

**Decision:** Run Agent Room on a dedicated Ubuntu Linux/amd64
`host_agentroom`. Terminate public HTTPS and proxy intentional routes through
Caddy on the independently managed `host_ingress`.

**Consequences:**

- only `host_ingress` is intentionally internet-facing
- public and private route policy must be explicit
- health, WebSocket, authentication, backup, and rollback behavior must be tested across both hosts
- private-upstream availability and edge security are early operational requirements

**Reference:** `memory-bank/build-deployment.md#Topology`

## 2026-07-28: Co-locate Caddy and Agent Room with a Loopback Upstream

**Status:** Accepted

**Context:** The production Caddy instance and Agent Room service run on the
same Ubuntu host. A second TLS boundary, private CA, client certificates, and
firewall policy between two processes on that host added failure modes without
adding a meaningful network trust boundary.

**Decision:** Terminate public HTTPS in Caddy and proxy to Agent Room over HTTP
on `127.0.0.1:8443`. Require Agent Room's application, administration, and
adapter listeners to bind to loopback. Deny private adapter and MCP routes at
Caddy, and trust forwarded client information only from an explicitly
configured loopback proxy.

**Alternatives:**

- Loopback mTLS was rejected because it duplicated Caddy's public TLS work and
  required unnecessary certificate lifecycle management.
- A second ingress host was rejected because the deployed topology is
  co-located and does not need a cross-host upstream.

**Consequences:**

- the Caddy site block is intentionally small
- no application TLS keys, private CA, client certificate, or host firewall
  rule is required for port 8443
- the service fails configuration validation if a listener or trusted proxy is
  not loopback
- deployment automation never edits the independently managed Caddyfile

**Reference:** `memory-bank/tasks/2026-07/280726_production-ingress-simplification.md`

## 2026-07-27: Build Once and Promote

**Status:** Accepted

**Context:** Development and production must differ by configuration, not by untracked builds.

**Decision:** Produce one immutable, checksummed artifact and promote that verified artifact to production on `host_agentroom`.

**Consequences:**

- production builds from uncommitted source are prohibited
- runtime configuration and secrets remain outside the artifact
- deployment records include provenance and checksum
- rollback retains a last-known-good artifact

**Reference:** `memory-bank/build-deployment.md#Build-Once-Promote`

## 2026-07-27: Go Modular Monolith on PostgreSQL

**Status:** Accepted

**Context:** The founding release needs transactional commands, immutable
events, deterministic projections, live browser updates, runtime adapters, and
MCP support without premature distributed-service operations.

**Decision:** Build a hexagonal Go modular monolith backed by PostgreSQL 18.
Use React and TypeScript for the browser client. Keep the domain and application
packages independent of HTTP, PostgreSQL, WebSocket, OIDC, and runtime SDKs.

**Consequences:**

- commands, events, projections, outbox records, and audit records share one
  transactional boundary
- the first deployment scales vertically without a message broker
- adapters and infrastructure remain replaceable ports
- a service split requires measured scaling or isolation evidence

**Reference:** `memory-bank/systemPatterns.md#Layered-Control-Plane-Architecture`

## 2026-07-27: Apple Silicon Development and Ubuntu Intel Production

**Status:** Accepted

**Context:** Development and debugging occur natively on `darwin/arm64`, while
the dedicated production hardware runs Ubuntu Linux on Intel.

**Decision:** Support native `darwin/arm64` development and debugging and
produce an immutable `linux/amd64` production artifact from the same source.
Run architecture-neutral tests on both targets. Run host-hardening and
performance gates on real Ubuntu Intel hardware.

**Consequences:**

- avoid CGO unless a measured requirement justifies it
- multi-architecture differences are explicit test dimensions
- emulated Linux results cannot prove production performance
- the exact checksummed Linux artifact is promoted to `host_agentroom`

**Reference:** `memory-bank/build-deployment.md#Build-Once-Promote`

## 2026-07-27: Native Hardened systemd Production Runtime

**Status:** Accepted

**Context:** The target is dedicated Ubuntu hardware and Agent Room must remain
operationally and permission-wise independent from Hermes.

**Decision:** Install versioned native Agent Room releases under a hardened
`systemd` service and AppArmor policy. Run PostgreSQL and pgBackRest as separate
managed services. Keep Hermes under its own Unix identity and lifecycle.

**Consequences:**

- releases switch through an atomic current-version pointer
- rollback restores the previous verified Agent Room artifact without touching
  Hermes
- Caddy reaches Agent Room only through its loopback HTTP listener
- development may use containers without making them the production runtime

**Reference:** `memory-bank/build-deployment.md#Runtime-Isolation`

## 2026-07-27: Apache License 2.0

**Status:** Accepted

**Context:** Public source distribution needs unambiguous rights for use,
modification, commercial distribution, contributions, and relevant patents.
Release archives also need to preserve dependency license and notice terms.

**Decision:** License Agent Room under Apache License 2.0 with Michael
Sitarzewski as the copyright holder. Treat intentionally submitted
contributions as Apache-2.0 unless explicitly designated otherwise. Ship the
project license, notice, reviewed third-party notices, full runtime dependency
license texts, and SPDX SBOM with release archives.

**Alternatives:**

- MIT was simpler but omitted an explicit patent grant.
- AGPL-3.0 would have imposed network copyleft and reduced permissive adoption.
- Proprietary or source-available terms would not meet the open-source goal.

**Consequences:**

- commercial and private use remain permitted under Apache-2.0 terms
- contributors and users receive an explicit patent grant with termination
- release packaging fails closed when required license material is absent
- dependency licenses remain an audited release input

**Reference:** `LICENSE`, `NOTICE`, `THIRD_PARTY_NOTICES.md`
