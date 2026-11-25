# Build and Deployment

**Status:** Shared-host cutover complete and healthy; Timeline release `086d31a` active on destination; local retirement and device acceptance pending

**Last updated:** 2026-11-24

## Post-Cutover Operations — 2026-11-24

The shared-host helper (`deploy/shared-host.sh`) now includes two repeatable
post-cutover modes in addition to the one-time migration lifecycle:

- `deploy/shared-host.sh backup <ABSOLUTE_OUTPUT_DIR>` — takes verified online
  custom-format `pg_dump` backups of both isolated databases (`app` and
  `identity`), archives site artifacts, snapshots the GitHub collector SQLite
  queue via `sqlite3.backup()`, and writes an owner-only SHA-256 `manifest.json`.
- `deploy/shared-host.sh upgrade <ABSOLUTE_ARTIFACT> <ABSOLUTE_PUBLIC_KEY>` —
  verifies a signed Linux/amd64 release archive, installs it immutably under
  `/srv/www/agentroom/releases/<revision>`, runs `agentroomctl migrate up`,
  flips `/srv/www/agentroom/current`, restarts `agentroom-web`, verifies
  `/readyz` returns HTTP 204, and automatically rolls back the symlink on
  readiness failure.

`agentroomctl doctor` now reports active, expiring-within-14-days, and expired
service tokens, and `agentroomctl service-token expiring [window-days]` outputs
machine-readable expiry JSON for rotation monitoring. `deploy/run-migration.py
check` preserves `cutover-complete` / `retired` checkpoint phases on transient
connectivity failures while still recording `remote_check_passed`.

## Current Operational Check — 2026-11-17

After the review below, Michael authorized source credential renewal through
December 31. The replacement is installed and expires 2026-12-31 23:59:59 Central;
its exact 99-project ingestion scope/actor and absolute expiry were verified in
the database. A genuine queued observation returned 202 and the old expired
token was revoked. Subsequent recovery backed up the queue, rescheduled only
401 retry timers under the collector lock, and reloaded the same publisher.
All 101,973 immutable records were preserved and the backlog has now drained:
pending 0, quarantined 0, all 99 projects delivering successfully. No queue reset
or remote activation. The early remote privilege preflight described below now
runs before any local shutdown; operator sudo input remains necessary.

The system walkthrough passes local readiness, HTTPS, OIDC login, 99-project
access and live connection. Remote staging never activated; local GitHub is
still disabled and the migration remains in fencing. Sentry's ingestion token
expired November 14: collection is current but per-project delivery is 401 and
roughly 100,500 observations are durably queued. Renew the exact-scoped source
credential and verify queue drainage before resuming cutover; do not reset the
queue or checkpoint. The commands below describe the prepared migration flow,
not proof that the destination is live. Actual review evidence stays ignored.

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

For updates, rerun the repository checks and build candidate assets separately
with `npm --prefix web run build -- --outDir /private/tmp/agentroom-candidate-web`.
The running daemon reads `web/dist` directly: a default build would change the
live frontend before verification. Build candidate native binaries separately
with the pinned Go toolchain, and preserve the existing binaries/assets/config.
After QA, atomically install the binaries and restart only `local.agentroom`;
confirm readiness before publishing the new assets and index. Keep old hashed
assets available for already open tabs. Native daemon changes require
`launchctl kickstart` after installation.
Frontend-only updates can retain the previously verified native binaries without
a daemon restart. Record frontend revision, dirty/clean status, file hashes and
activation phase separately from native provenance in ignored deployment/build
metadata; never imply both artifacts share a revision when only web is rebuilt.
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
The active configuration maps all 99 projects discovered and explicitly approved
on 2026-10-15. Existing project IDs and the durable queue are preserved. The
operator has the established full project permissions, while the publisher has
only `event:ingest,source:sentry` across those exact project IDs. Newly discovered
unmapped projects are reported as needing provisioning, not silently imported.

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

The original local token used a 30-day lifetime and expired November 14. Its
operator-approved replacement expires at the end of December 31, 2026 Central;
the exact UTC expiry, local expiry and current ID are in
`data/local/sentry/publisher-metadata.json`. Before expiry, create a replacement
with the same exact scopes, atomically replace the mode-0600 token file, verify
delivery, then revoke the previous token using the CLI. No collector restart is
needed to reload the token. Do not print or copy tokens into Memory Bank files.
Stop only this publisher with `launchctl bootout` for its own label. Agent Room
and Sentry remain independently supervised.

## Local GitHub Publisher

The read-only GitHub provider is supervised as `local.agentroom-github` and
uses the existing private adapter listener. Private configuration/state/token,
logs, backups and build evidence live in ignored `data/local/github/`.
Directories are owner-only; token/config files are mode 0600. Its initial mapping
covers this repository's existing Agent Room project, with only event:ingest
and source:github on its Agent Room token. Public GitHub access is anonymous;
no development GitHub credential is copied. Optional private GitHub read tokens
must be explicitly configured and renewed externally. Actual source-token ID
and expiry are in private `publisher-metadata.json` (90-day initial lifetime).

```sh
python3 -B adapters/github/agentroom_github.py \
  --config data/local/github/connector.toml status
launchctl print "gui/$(id -u)/local.agentroom-github"
launchctl kickstart -k "gui/$(id -u)/local.agentroom-github"
```

The default local poll is 900 seconds with a bounded request budget. `status`
is read-only. Do not run `once` concurrently with `watch`: one process owns the
state lock. Restart only this publisher to request a fresh immediate pass.
Preserve its state directory and repository binding through restarts/moves;
never clear the queue to repair transport. GitHub report freshness is independent
of Sentry's report in the same project. Existing source fields remain attributed
and cannot accept canonical completion or change human authority.

The pre-install snapshot includes native/web/configuration, both databases and
a consistent Sentry queue. Stop this publisher before rolling back to a daemon
without typed GitHub observation support, then restore the recorded artifacts.
Keep its queue for deliberate recovery. No new public ingress route or tailnet
mapping is needed. Final merged artifact provenance and activation phase are
recorded under `github_observations` in private deployment metadata. See
[GitHub observations](releases/github-observations.md).

## Local Participant Reporting

The approved agent coordination iteration adds an explicit metadata-only
`agentroomctl report` command. It requires a separately provisioned, exact-project
runtime token with `event:ingest,source:<runtime>`. Coordination capabilities are
separate and can be narrowed to requester or recipient actions. Sentry's
publisher credential remains source-only and must not acquire these permissions.

Reports run as the actual participant with its own native session identity.
They do not require database configuration, read conversation files, launch
workers, or install another service. Keep private state in a distinct mode-0700
directory per actor/project/runtime/instance/endpoint binding, and bearer files
mode 0600. Retry using `--retry` with the same binding; never delete the state to
work around an uncertain delivery. Only HTTP 202 clears queued observations.
Presence expires after five minutes. Report idle/closed at the end of actual
work, and revoke temporary acceptance credentials when testing is complete.
See `README.md#agent-coordination` for the command and
`releases/agent-coordination-workspace.md` for acceptance evidence.

## Shared-host Migration

Michael approved moving the local instance to the operator-designated shared
Linux host. Its canonical host guide requires additive, idempotent scripts that
Michael executes. Read-only SSH verified Linux/amd64, Docker access, the site
hardener and available proposed ports. Operator staging has now provisioned the
site and restored both snapshots; the application is not activated. No destination
change has been made by the assistant. Exact inventory, hostname, addresses, signing material, snapshots
and checkpoint data stay under ignored `data/local/migration/`.

This shared-host path is separate from the dedicated-host production installer
below. Do not run `install-host.sh` or `configure-instance.sh` on the shared host:
they manage native PostgreSQL and system-wide configuration. The selected move
keeps access private over Tailscale; no edge/DNS/shared Caddyfile changes occur.

### Prepared layout and authority

- Immutable verified release under `/srv/www/agentroom/releases/<source-revision>`
  with a `current` selector. Code belongs to the deployer and isolated site group.
- Native app and GitHub collector supervised as exactly `agentroom-web` and
  `agentroom-github`, running as `site_agentroom`; neither runs as the deployer.
- PostgreSQL and persistent Dex use the app-only `agentroom-host` Compose project
  and `agentroom-host-postgres` named volume. Other Docker projects and the host's
  MySQL/native PostgreSQL configuration are outside this deployment.
- App and identity provider retain loopback listeners. Only private Tailscale
  HTTPS ports 4445/4446 are added, with ownership checks refusing occupied routes.
  The adapter/admin/database ports remain loopback-only.
- GitHub's owner-only queue/token/config live in
  `/var/lib/agentroom-shared/github`, outside the recursively hardened code tree.
  The source adapter checks runtime ownership and 0700/0600, so the site hardener
  must not reset those private files to deployer ownership.
- The canonical site hardener runs unmodified. Pre-adding the site group avoids
  its Caddy restart branch. It still performs its documented graceful PHP-FPM
  reload; this is an operator-visible shared-host side effect.

`deploy/prepare-migration.py` snapshots both existing databases, artifacts and
the GitHub SQLite queue using database-aware backup, preserving source identities
and sequence numbers. It retains the existing Dex static subject, bcrypt hash,
client ID/secret, identity database and app/session credentials. A changed login
subject would create a different read-only account and lose the existing grants.
Preparation verifies the signed release; the target re-verifies signature,
provenance and full file inventory before provisioning. No development identity
fixture is promoted and no release is rebuilt on the destination.
Private baseline hashes cover local config, credentials, native binaries and
collector code before preparation reads them. Snapshot export checks the same
baseline before and after copying; a rotation or runtime change requires fresh
preparation rather than combining new database state with stale credentials.

### Operator execution

The actual private bundle is `data/local/migration/bundle`. From this checkout,
Michael runs the following commands in a normal interactive terminal; remote
scoped provisioning prompts for sudo when necessary:

Use an actual Terminal/iTerm window. The app's shell-command shortcut does not
provide an interactive terminal; the helper rejects it before any SSH operation
and now prints the specific terminal requirement. Staging is already complete;
the next operator step is `cutover`.

```sh
python3 -B deploy/run-migration.py --bundle data/local/migration/bundle stage
python3 -B deploy/run-migration.py --bundle data/local/migration/bundle cutover
python3 -B deploy/run-migration.py --bundle data/local/migration/bundle check
```

`stage` uploads the private bundle, provisions only the app's own slice and
rehearses database restoration. It does not activate the destination app or stop
local hosting. Before any local shutdown, `cutover` uses the reviewed local
shared-host helper inline over an operator SSH TTY to preflight the exact
previously uploaded staged manifest, signature, host identity, private listeners
and sudo access. It does not replace the staged helper or upload a new bundle
for that check. A failed preflight leaves local services and checkpoint phase
unchanged. Separate later SSH sessions may ask for sudo again; no password or
cross-TTY privilege cache is stored.

Once that preflight passes, `cutover` fences local app/GitHub/Sentry publishers, proves their
processes/listeners stopped, takes a fresh final snapshot, re-provisions/restores
that exact bundle, then activates and checks the destination. A rehearsal
snapshot cannot activate. Activation intent is durable before the remote call;
after uncertainty, the tool never silently reimports data or resumes the old app.
Both initial and final snapshots and upload history are retained privately.
If final provisioning stops after upload, use the same `cutover` command. The
runner verifies the retained rehearsal backup and final upload, then checks the
destination's markers and that the local writers are still stopped. Recovery
can continue from the rehearsal markers or the recorded stages of final
provisioning. An activation marker closes this provisioning recovery path.
Initial preflight and provisioning both require the canonical hardener at
`~/harden-site.sh` on the destination. [November recovery record](tasks/2026-11/181126_cutover-recovery.md)

After successful remote checks, tunnel and source resumption, the orchestrator
preserves the old deployment record and writes `data/local/deployment.json` as an
explicit pointer to `data/local/migration/orchestrator.json`, with remote active
URL/provenance and local retirement state. This avoids treating the old local
record as evidence that the app still runs on the workstation after a move.

After successful remote checks, a supervised private SSH tunnel forwards the
Mac's existing loopback adapter endpoint to the destination adapter. Only local
Sentry resumes; its source token, path bindings and delivery queue remain local.
Old app/GitHub hosting must stay disabled across Mac login. The tunnel must also
autoload at login so Sentry cannot restart without a persistent transport.

Open the new URL from the private bundle's `metadata.json`, sign in, verify the
existing project grants and source freshness, and accept browser/immersive
behavior on the actual device. Only then run:

```sh
python3 -B deploy/run-migration.py --bundle data/local/migration/bundle \
  --confirm REMOTE_BROWSER_ACCEPTED retire
```

`retire` repeats remote checks and stops only the old local Dex/PostgreSQL and
app-owned Serve routes. It keeps Sentry, its SSH tunnel, checkouts, private data
and recovery snapshots. Remote GitHub/app availability no longer depends on the
Mac; local Sentry freshness still ages while the Mac sleeps.

After remote writes begin, returning to the old database is a recovery migration,
not merely restarting the old binary. Preserve the destination's new data before
any rollback. The shared-host move does not inherit the dedicated-host native
pgBackRest setup; encrypted off-host backup destination/retention and a scheduled
restore-tested backup job remain operational work before claiming full production
backup acceptance. Current local snapshot rehearsal is not an off-host policy.

### Verification and current boundary

The migration suite has 76 tests, alongside the 53 existing source collector tests.
ShellCheck, Bash syntax, deployment/static/completeness and source secret checks
pass. Migration unit tests and shell validation are under `tests/deployment/` and
`deploy/shared-host.sh validate`. A private isolated Docker rehearsal restored
both real database dumps successfully and removed its disposable container/data.
Final activation, host restart recovery, actual source delivery over the tunnel,
device acceptance and retirement require the operator-run destination commands;
do not describe this host as live until those checks actually pass.

If cutover previously stopped during local fencing with `local writer remains
loaded`, the helper now waits for asynchronous launchd teardown and captured PID
exit instead of assuming `bootout` means immediate removal. A timeout still
forbids final export. The current checkpoint can resume with the same `cutover`
command; do not clear it or re-enable the old GitHub publisher.

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
