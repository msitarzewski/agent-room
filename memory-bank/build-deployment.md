# Build and Deployment

**Status:** Approved deployment architecture; production activation in progress

**Last updated:** 2026-07-28

## Topology

```
Public Internet
      │
      ▼
host_agentroom
Caddy: public TLS termination and private-route denial
      │
      ▼ HTTP on 127.0.0.1:8443
Agent Room service + persistent Agent Room data
```

Caddy is the only intentional public listener. Agent Room binds its public
application listener to loopback and is not directly reachable from another
host. Pip remains an independently managed Hermes participant connected
through the Hermes adapter.

## Environment Model

### Development

- isolated configuration, secrets, ports, and database
- native `darwin/arm64` Go and Node development
- multi-architecture containers for PostgreSQL, test OIDC, Caddy, and supporting
  integration services
- developer-controlled process lifecycle and native debugger support
- test adapters and fixtures may be enabled
- no production write credentials
- public Caddy route not required

### Production

- versioned immutable `linux/amd64` release archive on Ubuntu Linux `host_agentroom`
- production-only configuration and secrets
- dedicated persistent data and backup path
- hardened native `systemd` lifecycle and enforced AppArmor policy
- application HTTP accepted only on `127.0.0.1:8443`
- public requests enter through co-located Caddy

Development and production must never share a writable database.

## Build Once, Promote

The production artifact is the same `linux/amd64` archive verified by CI or the
approved local release process. The archive contains the `agentroomd` daemon,
the `agentroomctl` operator CLI, embedded browser assets and migrations, and
release metadata. It is installed into a versioned directory and activated
through an atomic `current` link.

Required metadata:

- application version
- source revision
- build timestamp
- dependency lock digest
- artifact checksum
- schema compatibility range

Do not build a different artifact on `host_agentroom`.

## Promotion Flow

1. Verify the source tree is committed and approved.
2. Run unit, integration, protocol, migration, security, and build checks.
3. Build one versioned artifact.
4. Verify its checksum and provenance.
5. Back up production data and current configuration references.
6. Transfer or pull the artifact to `host_agentroom`.
7. Validate production configuration without revealing secrets.
8. Stop accepting new mutating work or enter a migration-safe mode.
9. Apply forward-compatible migrations.
10. Start the new version.
11. Pass private readiness and health checks.
12. Pass smoke checks through co-located Caddy.
13. Mark the deployment successful.
14. Retain the last-known-good artifact and rollback metadata.

Every deployment produces a durable deployment record.

## Rollback Contract

Rollback triggers:

- process does not become ready
- private health check fails
- public smoke check fails
- migration verification fails
- critical security or data-integrity regression

Rollback:

1. Stop routing new traffic if required.
2. Stop the failed Agent Room version.
3. Restore the last-known-good artifact.
4. Reverse only migrations explicitly designed as reversible; otherwise restore the pre-deploy backup.
5. Start the previous version.
6. verify private health and public smoke paths.
7. record the failed deployment and rollback outcome.

Rollback must not roll back, recreate, or overwrite Hermes data.

## Caddy Boundary

The production Caddy configuration:

- terminate public HTTPS
- deny private adapter and MCP routes
- proxy remaining requests to `127.0.0.1:8443` over HTTP
- support WebSocket upgrades
- leave application authentication to Agent Room's OIDC relying party
- rely on Caddy's automatic certificate management and proxy headers

The application accepts forwarded client information only when the immediate
peer matches the explicit loopback trusted-proxy CIDR.

Keep private:

- administrative APIs
- migration and deployment endpoints
- debug routes
- raw telemetry
- detailed health diagnostics
- internal adapter callbacks unless explicitly authenticated

The public hostname and identity-provider details remain environment
configuration. The repository contains only a generic six-line Caddy example;
deployment automation never edits the host's Caddyfile.

## Authentication and Authorization

TLS is not authentication.

Before public availability:

- select an identity provider or strong application authentication mechanism
- require explicit authenticated identities
- disable anonymous control actions
- separate read, chat, approve, intervene, and administer permissions
- rate-limit authentication and mutating routes
- protect state-changing browser requests against cross-site attacks
- use secure, HTTP-only, same-site cookies if browser sessions are used
- define session expiry and revocation

Pip's service credentials must be scoped to the Hermes adapter capabilities it requires.

## Hermes Coexistence

Hermes and Agent Room:

- have separate service definitions
- have separate persistent data paths
- have separate credentials
- expose separate health checks
- upgrade independently
- roll back independently
- communicate only through documented hooks, APIs, MCP, or authenticated webhooks

Agent Room deployment automation must never run `hermes update`, rewrite Hermes configuration, or modify Hermes session storage.

## Backup and Restore

Back up:

- Agent Room authoritative database or event store
- artifact metadata not reproducible from source
- encrypted configuration references required for recovery
- deployment records

Do not treat a filesystem copy taken during active writes as a verified backup unless the storage engine documents it as safe.

Restore is not considered implemented until tested on isolated data.

## Selected Deployment Decisions

- application artifact: immutable checksummed `linux/amd64` release archive
- service supervisor: native `systemd`
- host confinement: dedicated Unix identity plus AppArmor and systemd sandboxing
- database: PostgreSQL 18
- backup mechanism: pgBackRest with WAL archiving and verified isolated restore
- nonsecret configuration: `/etc/agentroom/agentroom.conf`
- secrets: systemd credential files referenced through `AGENTROOM_*_FILE`
- public upstream: loopback HTTP listener on `127.0.0.1:8443`
- private administration: loopback listener on `127.0.0.1:9090`
- health: minimal public `/healthz`; private `/livez` and `/readyz`
- migrations: embedded and invoked with `agentroomctl migrate`

## Open Operational Values

- deployment runner and secure access path to `host_agentroom`
- concrete OIDC provider and client registration
- encrypted off-host backup repository and retention
- secret recovery procedure
- log and telemetry retention

These are environment-specific values and must be supplied or discovered from
authorized host configuration before production activation.
