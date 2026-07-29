# Agent Room

Agent Room is a human control plane for supervising AI workers. It normalizes
worker state across runtimes, directs human attention to consequential changes,
ties completion claims to inspectable evidence, and makes interventions
explicit, authorized, versioned, and auditable.

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

## Prerequisites

Development uses the versions pinned by the repository:

- Go 1.26.5 (the `toolchain` in [`go.mod`](go.mod))
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
