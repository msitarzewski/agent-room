# Agent Room

Agent Room is a shared coordination workspace for agents. It brings work,
ownership, requests, handoffs, and evidence into one attributable record across
runtimes. Humans can observe, participate, steer, and decide exceptions; routine
coordination should reach the responsible agents. Commands remain explicitly
authorized, versioned, idempotent, and auditable.

This repository is an implemented product foundation, not a finished autonomous
worker platform. It includes the control-plane daemon and CLI, PostgreSQL
coordination record, browser interface, OIDC authentication, project
authorization, REST/WebSocket/MCP boundaries, artifact integrity, tests, and a
signed Linux deployment path. Hermes hook normalization and an API client are
present, but the repository does not yet include an isolated production worker
service or a production connector process that runs the Hermes/Pip integration.

Read [boot.md](boot.md) for the product and technical design. Read
[deploy/README.md](deploy/README.md) before building or operating a production
installation.

## Current boundaries

- Agent Room owns accepted commands, normalized coordination state, attention,
  approvals, artifact metadata, and audit records.
- Git, CI, Hermes, model providers, and other source systems remain authoritative
  for the facts they originate.
- Browser chat is a coordination surface. A message does not silently become a
  control-plane command.
- The public application listener, loopback administration listener, and
  loopback adapter listener are separate trust boundaries.
- Production in-process Codex and Claude execution is deliberately disabled.
  Production configuration rejects managed-runtime binaries until execution
  moves into a separately isolated worker service.

## Architecture

```text
Browser
  │ OIDC session + CSRF + project capabilities
  ▼
agentroomd public listener
  ├── REST control plane and resource projections
  ├── WebSocket event stream
  ├── immutable artifact upload/download
  └── production SPA
          │
          ▼
PostgreSQL 18
  commands · events · projections · outboxes · audit · sessions

Private adapter listener (loopback or encrypted tunnel only)
  ├── service-token event ingestion
  └── scoped MCP server

Loopback administration listener
  └── liveness and readiness
```

The Go daemon is the only writer of canonical state. Accepted commands produce
immutable events and transactional projections. PostgreSQL outboxes keep
realtime publication and supported control delivery recoverable. The React/Vite
application consumes the same versioned HTTP contract documented in
[`api/openapi/agent-room.v1.yaml`](api/openapi/agent-room.v1.yaml); adapter
events use [`api/arp/event-envelope.schema.json`](api/arp/event-envelope.schema.json).

## Spatial observatory

Open **Observatory** in the sidebar, or visit `/workspace`. The portfolio groups
projects into **Coordination**, **Work recorded**, **Quiet**, and **Coverage
unknown**. Expand a region to reveal named projects in pages of twelve. Actual
requests, blockers, reviews, ownership, and agent presence supply the signals;
repository activity never implies a running worker. Quiet requires a complete,
recent source collection and authorized coordination coverage.

Select a project to unfold its tasks and recorded dependencies, then select a task to inspect runs,
evidence, artifacts, review state, and source observations. Project and task
selections are shareable through the URL. The searchable directory supports
keyboard navigation and remains available when graphics cannot render.

The desktop scene uses WebGPU when available and falls back to WebGL 2. Drag to
explore, scroll to move closer, and use **Recenter view** to restore framing.
Motion reflects reported running work and respects reduced-motion preferences.
Attention remains within reach; source completion is separate from accepted
completion. Review decisions require confirmation, and controls retain the
existing project capabilities, resource versions, and command audit.

In a secure browser with immersive WebXR support, **Enter immersive** opens the
same project and selection using a dedicated WebGL 2 renderer. Select with the
device's natural input, pinch the handle to reposition the workspace, and use **Exit
immersive** or the headset's system control to return. Real Vision Pro comfort,
input, and frame pacing still require acceptance on the exact visionOS build.
The development emulator and sample environments are omitted from production.

The [spatial release checklist](memory-bank/releases/spatial-workspace.md) and
[coordination release](memory-bank/releases/agent-coordination-workspace.md)
record implementation, verification, and device acceptance requirements.

## Agent coordination

Agents use the private, project-scoped MCP surface to
`list_coordination_requests`, `create_coordination_request`, and
`act_coordination_request`. Requests have kind `help`, `context`, `handoff`, or
`review`, an authenticated owner, a named agent or human recipient, and optional
same-project task/evidence references. A human recipient requires an explicit
escalation reason. The named recipient responds; the requester resolves or
cancels. An authorized operator can intervene with its own attribution.

Each mutation needs an idempotency key; actions also need the current resource
version. Narrow machine capabilities are `coordination:read`,
`coordination:create`, `coordination:respond`, `coordination:resolve`, and
`coordination:cancel`. Request bodies are recipient-scoped in resource reads,
summaries, event replay, and live streams. Full project readers can observe all
requests. A response does not transfer ownership, accept completion, or execute
work. The browser uses the equivalent typed HTTP contract with its normal
session, CSRF protection, and project permissions.

`agentroomctl report` explicitly reports real agent/session metadata without
loading daemon configuration or requiring database credentials:

```sh
agentroomctl report \
  --project-id <authorized-project> --runtime codex \
  --agent-id <token-actor-id> --instance-id <stable-instance-id> \
  --display-name '<participant-name>' \
  --state-dir /absolute/private/reporter-state \
  --token-file /absolute/private/reporter-token \
  --adapter-url http://127.0.0.1:54491
```

Use a separate project-scoped token with `event:ingest` and `source:codex`
(or `source:claude` / `source:hermes`). Codex uses its actual `CODEX_THREAD_ID`;
other callers supply `--session-id`. The reporter never reads conversations or
starts workers. Provide `--run-id` and `--run-status` only for an actual bounded
native run. Agent presence expires after five minutes; repeat reports during
real work and report `--status idle --session-status idle` when it ends.

Ordinary runtime source credentials accept only typed `agent.observed`,
`session.observed`, and `run.observed` events, bound to the authenticated actor.
Legacy codec lifecycle exports must be mapped to this typed contract before
connecting a provider-hook service. They cannot update human tasks, approvals,
or another participant's runs through generic event names.

State directories require mode 0700; token/state files require mode 0600 and
current-user ownership. Observations persist before transmission; `--retry`
drains the original queued bodies without creating activity. Only HTTP 202
acknowledges delivery. Permanent invalid or expired observations stay
quarantined for review. Preserve the directory across restarts, and never
commit runtime state or bearer credentials. This explicit reporter is not an
installed provider-hook service or autonomous worker executor.

## Prerequisites

Development uses the versions pinned by the repository:

- Go 1.26.9 (the `toolchain` in [`go.mod`](go.mod))
- Node.js 24.8.0 and npm 11.6.0
- Docker with Compose for PostgreSQL 18 and the local Dex provider
- Git, `curl`, and `jq`
- Chromium and WebKit installed through Playwright for native macOS browser
  tests

The signed production builder additionally requires Linux, GNU tar, `file`,
`gzip`, `sha256sum`, Syft, Cosign, a signing key, a clean committed checkout,
and a PostgreSQL test database. The emitted application binaries are
Linux/amd64 ELF artifacts; they do not run natively on Apple Silicon.

## Install and verify the source tree

```sh
npm --prefix web ci --ignore-scripts
npm --prefix web exec -- playwright install chromium webkit

go test -race ./...
npm --prefix web run typecheck
npm --prefix web run lint
npm --prefix web run api:lint
npm --prefix web test
npm --prefix web run build
npm --prefix web run test:e2e
```

On native Darwin, the default browser gate runs Chromium and WebKit. CI runs
Chromium, Firefox, and WebKit in the pinned Linux Playwright image with:

```sh
npm --prefix web run test:e2e:all
```

The frontend coverage gate is enforced by `web/vitest.config.ts`. The
race-enabled backend coverage and integration gate is:

```sh
AGENTROOM_TEST_DATABASE_URL='postgres://agentroom:<password>@127.0.0.1:<port>/agentroom_test?sslmode=disable' \
  tests/security/backend-quality.sh
```

For an isolated, disposable PostgreSQL 18 and Dex regression—including
PostgreSQL persistence and the HTTP contract—run:

```sh
tests/security/compose-postgres18.sh
```

The static deployment, artifact, authorization, and completeness gates are:

```sh
tests/security/deployment-static.sh
tests/security/artifact-fail-closed.sh
tests/security/completeness-scan.sh
tests/security/source-authorization-contract.sh
tests/security/static-security.sh
tests/security/license-compliance.sh
```

Active penetration testing is never run implicitly. It requires explicit
authorization and an isolated target; see
[deploy/README.md](deploy/README.md#development-and-security-verification).

## Native Apple Silicon development

For the already configured local tailnet installation, open the URL in
`data/local/deployment.json` and sign in with the `username` and `password` in
`data/local/secrets/login.json`. These are private generated credentials. The
app is served over Tailscale HTTPS port 4445, with its identity provider on port
4446, using a supervised native daemon and persistent local containers. See the
[local deployment runbook](memory-bank/build-deployment.md#local-tailnet-deployment)
for status, restart, update, and stop procedures. This is also the WebXR entry
point for Vision Pro Safari. Michael has confirmed immersive operation; detailed
interaction, comfort, recovery, and measured frame pacing remain device checks.

The supported workstation shape is a native Go daemon and frontend build, with
PostgreSQL and Dex in loopback-published containers. Do not use the production
Ubuntu installation scripts on macOS.

Create local-only Compose secrets:

```sh
mkdir -p deploy/compose/secrets data/dev/artifacts data/dev/workspaces bin
chmod 700 deploy/compose/secrets
printf '%s\n' 'agentroom-local-only' > deploy/compose/secrets/postgres-password
printf '%s\n' \
  'postgres://agentroom:agentroom-local-only@127.0.0.1:55432/agentroom?sslmode=disable' \
  > deploy/compose/secrets/database-url
printf '%s\n' 'local-session-secret-with-at-least-32-bytes' \
  > deploy/compose/secrets/session-secret
printf '%s\n' 'agentroom-dex-local-only' \
  > deploy/compose/secrets/oidc-client-secret
chmod 600 deploy/compose/secrets/*
```

Start only the local infrastructure:

```sh
docker compose -f deploy/compose/compose.yaml up --detach --wait postgres dex
npm --prefix web run build
go build -o bin/agentroomd ./cmd/agentroomd
go build -o bin/agentroomctl ./cmd/agentroomctl
```

Export the native development configuration:

```sh
export AGENTROOM_DEV=true
export AGENTROOM_HTTP_ADDR=127.0.0.1:58443
export AGENTROOM_ADMIN_ADDR=127.0.0.1:59090
export AGENTROOM_ADAPTER_ADDR=127.0.0.1:59091
export AGENTROOM_PUBLIC_URL=http://127.0.0.1:58443
export AGENTROOM_ALLOWED_ORIGINS=http://127.0.0.1:58443
export AGENTROOM_OIDC_ISSUER=http://127.0.0.1:5556/dex
export AGENTROOM_OIDC_CLIENT_ID=agentroom-local
export AGENTROOM_OIDC_REDIRECT_URL=http://127.0.0.1:58443/api/v1/auth/callback
export AGENTROOM_ARTIFACT_DIR="$PWD/data/dev/artifacts"
export AGENTROOM_WEB_DIR="$PWD/web/dist"
export AGENTROOM_WORKSPACE_ROOT="$PWD/data/dev/workspaces"
export AGENTROOM_DATABASE_URL_FILE="$PWD/deploy/compose/secrets/database-url"
export AGENTROOM_SESSION_SECRET_FILE="$PWD/deploy/compose/secrets/session-secret"
export AGENTROOM_OIDC_CLIENT_SECRET_FILE="$PWD/deploy/compose/secrets/oidc-client-secret"
```

Initialize one development project, then start the daemon:

```sh
bin/agentroomctl migrate up
bin/agentroomctl bootstrap \
  local-project 'Local Agent Room' \
  CiQyMzhiNmY3Yi0xN2JkLTRiMGQtYTE5NS0yNmU3MjViNzc2Y2ESBWxvY2Fs \
  operator@agentroom.local
bin/agentroomd
```

Open <http://127.0.0.1:58443>. The local Dex login is
`operator@agentroom.local` with password `agentroom-local-only`. These are
checked-in development fixtures, never production credentials. The bootstrap
command is a one-time initialization for the chosen project ID. Its identity
argument is the exact provider-issued OIDC `sub`, which Dex connector-scopes;
it is deliberately not the raw `staticPasswords.userID` from the Dex config.

When finished:

```sh
docker compose -f deploy/compose/compose.yaml down
```

Docker Desktop host-network behavior varies by configuration. The native path
above depends only on loopback-published PostgreSQL and Dex ports; the optional
Compose `app` profile is intended for the Ubuntu target.

## Linux/amd64 release and production promotion

Production uses one caller-assigned release identifier and one verified
artifact promoted through environments. The repository does not assign a
product release version.

From a clean committed checkout on a Linux builder:

```sh
deploy/build-release.sh \
  --version <assigned-release-version> \
  --output-dir dist \
  --cosign-key <ci-cosign-private-key>
```

The builder reruns the locked frontend and backend gates, cross-builds
`agentroomd` and `agentroomctl` for Linux/amd64, embeds Git provenance, creates
an SPDX SBOM and exact file manifest, and emits a signed archive plus checksum.

Host preparation, co-located Caddy TLS termination, loopback HTTP proxying,
PostgreSQL/pgBackRest, promotion, smoke testing, backup records, and isolated
restore drills are defined in
[deploy/README.md](deploy/README.md). The production transition is:

```sh
sudo deploy/deploy.sh \
  --artifact <agentroom-release-linux-amd64.tar.gz> \
  --public-key <cosign-public-key.pem> \
  --public-url https://<public-host>
```

Rollback changes the verified application pointer and restores the previous
runtime state; expand-only migrations are not silently reversed:

```sh
sudo deploy/rollback.sh --public-url https://<public-host>
```

Use an explicit installed version only when the operator has selected it:

```sh
sudo deploy/rollback.sh \
  --version <installed-version> \
  --public-url https://<public-host>
```

## GitHub observations

The independent [GitHub monitor](adapters/github/agentroom_github.py) reads
issues, pull requests, reviews and current-head check/status results through
GitHub's REST API. It performs outbound GET requests only. Public repositories
need no GitHub credential; private repositories require an explicitly configured,
owner-only read token file. Expiring GitHub App tokens must be refreshed outside
the collector. It never reads the development CLI's credential or writes GitHub.

Copy [the configuration example](deploy/config/github-connector.toml.example)
to private storage and explicitly map each repository to an existing Agent Room
project. Provision an Agent Room service token with only `event:ingest` and
`source:github` for those project IDs. This publisher cannot create projects,
change memberships, approve completion, or change task ownership.

```sh
python3 -B adapters/github/agentroom_github.py \
  --config /path/to/private/github-connector.toml watch
```

Python 3.11+ and the complete checkout are required; delivery is shared with
Sentry in `adapters/source.py`. `watch` polls every 900 seconds by default;
`once` collects and sends a bounded batch; `status` reads persisted health.
Exit 2 means delivery remains pending and exit 1 reports collection/delivery
errors or quarantine. Keep state directories mode 0700 and token files mode
0600. Preserve the durable state directory across restarts and host moves.

Repository numeric identity is bound to its project on first verified access.
Issues and PRs keep distinct stable identities across close/reopen and updates.
Pagination, conditional requests, API request budgets, rate-limit backoff and
partial coverage are explicit. Missing data never deletes tasks or establishes
successful checks. Check results refer to the current PR head; branch-protection
requirements and merge eligibility are not inferred. Observed failures or
requested changes surface source blockers for agent triage; review signals do
not accept a canonical completion claim or automatically escalate to a human.
Closed and merged PRs are distinct source facts. Historical CI on closed PRs
is not repeatedly refreshed.

GitHub and Sentry have independent source report/freshness records in the same
project, retaining canonical names and human-owned fields. The monitor uses the
same private typed observation endpoint described below. When rolling back to
a daemon without GitHub support, stop this publisher before restoring binaries;
retain its queue for deliberate recovery. See the
[GitHub release record](memory-bank/releases/github-observations.md).

## Sentry observations

The independent [Sentry connector](adapters/sentry/agentroom_sentry.py) reuses
Sentry's read-only scanner and publishes project and task observations through
the private adapter endpoint. Sentry's own board, watcher, history, and backups
continue operating independently. Install the connector from this checkout on
the source workstation, using Sentry's existing Python environment (Python 3.11
or newer). It is separate from the daemon/web release archive.

Copy [the configuration example](deploy/config/sentry-connector.toml.example)
to private runtime storage. Resolve each source path to an operator-provisioned
project ID. Use `agentroomctl project ensure` and `membership grant` for setup;
the publisher cannot provision projects or memberships. A service token can
cover a comma-separated list of exact project IDs with only `event:ingest` and
`source:sentry`; keep its file owner-only and rotate it before expiry. Configure
the private adapter URL with HTTPS, or loopback HTTP on the same host.

```sh
/path/to/sentry/.venv/bin/python -B adapters/sentry/agentroom_sentry.py \
  --config /path/to/private/connector.toml watch
```

`watch` collects every 300 seconds by default and drains queued observations
between polls. `once` performs a bounded collection/delivery pass; exit 2 means
pending delivery remains, and exit 1 reports errors or quarantined observations.
`status` reports backlog, provisioning needs, and collection/delivery errors.
Queue records are durable before transmission and retries preserve event IDs,
sequences, payloads, and collection times. Missing or unreadable source records
never delete existing tasks. Unmapped projects require operator provisioning;
permanent invalid records remain quarantined for review. Preserve the state
directory across restarts and host moves to retain identity and ordering.

Give an observation publisher only `event:ingest` and `source:sentry` for its
chosen projects. Send `POST /api/v1/ingest?project_id=<project-id>` to the private
adapter listener with its bearer service token. The body contains an `events`
array; each event supplies `project_id`, `type: "task.observed"`,
`subject_type: "task"`, a nonempty `subject_id`, `source_system: "sentry"`, a
stable `source_event_id`, and a positive monotonically increasing
`source_sequence` for that source task. The server derives the canonical task
ID from the project and the payload's `external_id`; the supplied subject ID
does not choose an arbitrary resource.

The payload requires `external_id`, `title`, and `status`. It optionally includes
`description`, an HTTP(S) `external_url`, and `dependencies` containing external
task IDs from the same project. Unknown payload fields and resource replacements
are rejected. Use the current observation time for `occurred_at`, or omit it
for server time; the endpoint accepts at most 30 days of clock skew into the
past and five minutes into the future. Retries retain the source event ID;
later observations advance the sequence. See the precise
[TaskObservation contract](api/openapi/agent-room.v1.yaml).

New observations create supervised tasks in `inbox`. Later source updates
retain human-owned fields and record the latest observation separately. A
blocked source task creates deduplicated attention, and a newer unblocked
observation resolves only that source-owned item. A human resolution stays
resolved until a new unblocked-to-blocked episode. Source `Done` never accepts a
completion claim.

`project.observed` reports the actual collection time, source name/path/status,
scan status (`complete`, `partial`, or `unreadable`), and task count, including
successful zero-task scans. It updates a separate source report and preserves
the operator's project name. Workspace and overview responses distinguish
`last_collected_at`, `last_received_at`, and `last_successful_collection_at`;
queued delivery cannot make an old scan appear fresh. The browser labels project
activity separately from connection opening and heartbeat receipt.

`GET /api/v1/workspace?project_id=<project-id>` returns one authorized,
transactionally consistent snapshot of the resources and source receipt times
used by the scene. It requires read access to every included resource kind;
partial graphs fail closed. WebSocket catchup and resync messages refresh the
snapshot. No private adapter credentials belong in browser configuration.

## Hermes and Pip

Pip is a persistent Hermes participant and is distinct from any physical host
running Hermes or Agent Room. Hermes remains authoritative for Pip's memory,
skills, conversations, and native sessions. Agent Room stores normalized
coordination records and source links; it does not read or replace Hermes
internals.

Hermes and Agent Room must use separate operating-system users, services,
credentials, listeners, and data directories. Pip should eventually
participate through least-privilege service-token ingestion and the private MCP
surface. Those adapter endpoints bind to loopback and are reached remotely only
through an operator-managed encrypted tunnel; Caddy intentionally does not
publish them.

No production Hermes connector service or isolated runtime worker ships in this
foundation. Until those boundaries exist and are reviewed, production Agent
Room observes and coordinates externally reported work but does not launch
Codex, Claude, Hermes, or other worker processes.

## Repository map

- [`cmd/agentroomd`](cmd/agentroomd) — control-plane daemon
- [`cmd/agentroomctl`](cmd/agentroomctl) — migrations and administration
- [`internal/app`](internal/app) — authorization-aware application service
- [`internal/postgres`](internal/postgres) — durable record, projections, and
  outboxes
- [`internal/httpapi`](internal/httpapi) — public, adapter, realtime, and MCP
  boundaries
- [`internal/adapters`](internal/adapters) — Codex, Claude, and Hermes-native
  normalization/client foundations
- [`adapters/sentry`](adapters/sentry) — independent durable source collector
- [`web`](web) — browser control room
- [`api`](api) — HTTP and ARP contracts
- [`db`](db) — embedded migrations and SQL queries
- [`tests`](tests) — API, completeness, security, and penetration gates
- [`deploy`](deploy) — reproducible release, host hardening, promotion,
  rollback, backup, and restore
- [`third_party`](third_party) — redistributable runtime dependency licenses
- [`memory-bank`](memory-bank) — project decisions and working context for
  collaborating agents

## Contributing

Contributions are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md) for the
development gates, architecture expectations, and inbound licensing terms.

## Security and support

Report vulnerabilities privately as described in [SECURITY.md](SECURITY.md).
Use repository issues for reproducible defects and focused feature proposals;
this independent project does not currently provide a commercial support SLA.

## License

Agent Room is licensed under the [Apache License 2.0](LICENSE). Copyright 2026
Michael Sitarzewski. Runtime dependency attributions and license texts are in
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) and [`third_party`](third_party).
