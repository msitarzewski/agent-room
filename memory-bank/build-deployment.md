# Build and Deployment

**Status:** Local tailnet deployment running; signed Linux production promotion separate

**Last updated:** 2026-10-14

## Local Tailnet Deployment

Michael authorized MiradorXR's local loopback + Tailscale Serve HTTPS pattern.
This macOS deployment extends the native Apple Silicon procedure in `README.md`
and existing `deploy/compose/compose.yaml` PostgreSQL/Dex definitions. It runs
the production web build with `AGENTROOM_DEV=false` and normal OIDC authorization.

| Component | Local binding | Tailnet HTTPS |
| --- | --- | --- |
| Native application | `127.0.0.1:4445` | dedicated port 4445 |
| Dex identity provider | `127.0.0.1:5557` | dedicated port 4446, `/dex` issuer |
| PostgreSQL | `127.0.0.1:55440` | none |
| Administration | `127.0.0.1:54490` | none |
| Private adapter | `127.0.0.1:54491` | none |

Tailscale Serve owns HTTPS and WebSocket proxying. The application trusts only
loopback proxies and its configured HTTPS origin. MiradorXR's port 4444 and all
other existing tailnet mappings are retained; Funnel is not enabled.

Actual URLs are in `data/local/deployment.json`. Operator credentials are in
`data/local/secrets/login.json`; use the `username` and `password` values at the
OIDC login screen. These ignored files stay private, outside Git and Sentry docs.
`data/local/agentroom.conf` references private secret files. Application and Dex
use separate non-superuser database roles and databases; Dex signing keys and
identity sessions persist in PostgreSQL. The Compose project is `agentroom-local`.

### Local Lifecycle

The native `bin/agentroomd` is supervised by per-user launch agent
`local.agentroom`. Its installed LaunchAgents entry links to
`data/local/local.agentroom.plist`. It starts at user login and restarts after
failure; database containers use `unless-stopped`. Docker Desktop, Tailscale,
and an awake workstation are required for remote headset access.

Check or restart the existing installation from the repository root:

```sh
docker compose -f data/local/compose.json up --detach --wait postgres dex
launchctl print "gui/$(id -u)/local.agentroom"
launchctl kickstart -k "gui/$(id -u)/local.agentroom"
curl --fail http://127.0.0.1:54490/readyz
tailscale serve status
```

The readiness success response is HTTP 204. For a browser document check, send
`Accept: text/html` to the HTTPS `/workspace` URL. An unauthenticated workspace
API read must return 401. Login preserves the selected project/task deep link.
Diagnostic logs and browser/restart evidence are in `data/local/logs/`.

For updates, rerun the repository checks, rebuild `web/dist`, and rebuild native
binaries with the pinned Go toolchain. The daemon reads the web distribution
from disk; native daemon changes require `launchctl kickstart` after building.
Use the existing CLI with `--config data/local/agentroom.conf` for migrations,
doctor, and scoped administration. Keep private snapshots of both `agentroom`
and `agentroom_identity` databases before changes; the initial restart snapshots
are in `data/local/backups/`. This local instance has not installed the Linux
pgBackRest/off-host backup policy described below.

To stop this instance while retaining its data:

```sh
launchctl bootout "gui/$(id -u)/local.agentroom"
docker compose -f data/local/compose.json stop
tailscale serve --https=4445 off
tailscale serve --https=4446 off
```

To reload a stopped launch agent, use `launchctl bootstrap "gui/$(id -u)"
data/local/local.agentroom.plist`, then restore only its two Serve mappings to
the loopback app and Dex ports:

```sh
tailscale serve --bg --https=4445 --yes http://127.0.0.1:4445
tailscale serve --bg --https=4446 --yes http://127.0.0.1:5557
```

Never reset all Serve routes or remove database
volumes to restart this instance. Preserve the checkout, private runtime files,
and LaunchAgents link. Sentry's independent services are unaffected.

Physical Vision Pro acceptance and signed Linux promotion remain separate from
the local WebXR build. Read `releases/spatial-workspace.md` for their checklist.

The operator-designated remote machine is the eventual deployment home. This
local installation is a staging step for headset testing. Its actual host
identifier stays in `data/local/deployment.json`; verify that host's platform
before producing the promoted artifact and activating the remote service.

## Local Sentry Publisher

The independent publisher runs from this checkout with Sentry's existing Python
environment. It uses Sentry's scanner read-only; its current board, watcher,
history, backups, and source configuration are unchanged. The connector is
installed separately from the daemon/web release archive.

Private configuration is `data/local/sentry/connector.toml`, durable SQLite state
is `data/local/sentry/state/`, and publisher credentials are under
`data/local/sentry/secrets/`. Directories are owner-only, files mode 0600.
The active configuration maps the existing `agent-rooms` project only. Additional
discovered projects are reported as needing provisioning, not silently imported.
Full portfolio setup requires the operator's separate scope decision.

```sh
/path/to/sentry/.venv/bin/python -B adapters/sentry/agentroom_sentry.py \
  --config data/local/sentry/connector.toml status
launchctl print "gui/$(id -u)/local.agentroom-sentry"
launchctl kickstart -k "gui/$(id -u)/local.agentroom-sentry"
```

The launch agent uses `watch`, collects every 300 seconds, and drains pending
delivery between collections. Its private plist and logs are in
`data/local/sentry/`. Preserve its state directory and source instance/path
bindings through restart or host migration. Never reset the queue to repair
transport or authorization failures: immutable queued observations are retried
with their original event IDs, sequences, and collection times. `status` exposes
pending delivery, quarantined records, provisioning needs, and safe error codes.
Unmapped/unreadable/window-expired sources do not delete existing records.

Provision additional projects using `agentroomctl project ensure` and the
existing `membership grant` operator commands before changing the mapping.
Reconciliation preserves canonical names, and unchanged grants do not rotate
sessions. Actual access changes rotate sessions as before. Publisher tokens
use exact project IDs and only `event:ingest,source:sentry`; no wildcard,
administration, database access, or resource replacement is granted.

The local token has a 30-day lifetime; its actual expiry and ID are in
`data/local/sentry/publisher-metadata.json`. Before expiry, create a replacement
with the same exact scopes, atomically replace the mode-0600 token file, verify
delivery, then revoke the previous token using the CLI. No collector restart is
needed to reload the token. Do not print or copy tokens into Memory Bank files.
Stop only this publisher with `launchctl bootout` for its own label. Agent Room
and Sentry remain independently supervised.

## Production Topology

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
