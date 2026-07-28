# Decisions

**Last updated:** 2026-07-27

Only accepted decisions belong here. Open technical choices remain in `techContext.md` and `build-deployment.md`.

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

**Status:** Accepted

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
- Caddy reaches only the authenticated encrypted upstream
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
