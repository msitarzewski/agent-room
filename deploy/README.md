# Agent Room deployment

Agent Room is deployed independently from agent runtimes. The supported
production target is an Ubuntu Linux/amd64 host, PostgreSQL stores durable
state, and Caddy is the only public ingress. The default topology co-locates
Caddy and Agent Room: Caddy terminates public TLS and proxies HTTP to the
loopback-only application listener. These installation scripts must not be run
on a Darwin development workstation.

## Supported production shape

- Ubuntu Linux on Intel x86-64 hardware
- versioned releases under `/opt/agentroom/releases/<version>`
- atomic `/opt/agentroom/current` symlink
- systemd units with encrypted credentials and AppArmor confinement
- PostgreSQL 18 with pgBackRest backups
- Caddy TLS termination and HTTP proxying to `127.0.0.1:8443`
- adapter APIs on `127.0.0.1:9091`, reachable only through an approved
  private encrypted tunnel

The application and Hermes must have separate Unix users, state directories,
credentials, listeners, and service units. None of these scripts install,
restart, or read Hermes.

The service-token adapter endpoints (`/api/v1/ingest`, `/api/v1/mcp`, and
`/api/v1/adapters/*`) are deliberately absent from the public application
handler. Caddy explicitly returns 404 for those paths. A remote adapter must
use an operator-managed SSH or WireGuard tunnel that terminates at the
loopback-only adapter listener. Do not publish port 9091 or add those paths to
Caddy.

## Release build and verification

Run the production builder only from a clean committed checkout:

```sh
deploy/build-release.sh \
  --version <version> \
  --output-dir dist \
  --cosign-key <ci-cosign-private-key>
```

The builder derives the exact revision and reproducible timestamp from Git,
runs the locked frontend and backend gates, embeds clean VCS metadata in both
Linux/amd64 binaries, records matching frontend provenance, and calls the
packager. The output includes a signed archive, checksum, exact file manifest,
SPDX SBOM, project license, and third-party license material. Production
packaging and deployment reject unknown or forged
source revisions, dirty binaries, mismatched frontend provenance, unsigned or
altered content, wrong-platform binaries, path traversal, links, and
unmanifested files. `deploy/package-release.sh --development-unsigned` remains
available for local packaging and is never accepted by production deployment.

## Prepare `host_agentroom`

For a new Ubuntu/amd64 host, install verified production prerequisites and the
host files in one operation:

```sh
sudo deploy/install-host.sh --bootstrap --apply
```

Bootstrap uses the official PostgreSQL APT repository for the host's Ubuntu
codename, verifies the repository signing-key fingerprint, installs PostgreSQL
18, pgBackRest, and the deployment-gate utilities, and installs a
checksum-pinned Cosign package. It binds PostgreSQL to loopback and starts with
a conservative 256 MiB shared-buffer budget and 50-connection limit. It does
not create database or OIDC credentials and does not start Agent Room.

On a prepared host where prerequisites are already managed separately, install
only the Agent Room host files:

```sh
sudo deploy/install-host.sh --apply
```

Create an OIDC client in the chosen provider with the redirect URI
`https://<public-host>/api/v1/auth/callback`. Then configure the instance:

```sh
sudo deploy/configure-instance.sh \
  --public-url https://<public-host> \
  --oidc-issuer https://<private-oidc-issuer> \
  --oidc-client-id <client-id> \
  --apply
```

The configurator reads the OIDC client secret without echo, creates the local
database and random database/session credentials, encrypts all application
secrets with `systemd-creds`, writes strict nonsecret application config, and
initializes pgBackRest. Existing encrypted credentials are preserved. For
unattended provisioning, pass a root-only regular file with
`--oidc-client-secret-file` and remove it after the command succeeds.

The parser accepts only documented `AGENTROOM_KEY=value` entries and treats
unknown or duplicate keys as fatal. Forwarded client addresses are honored
only when the immediate peer matches the configured loopback proxy network.

## Configure Caddy

The configurator deliberately does not edit a shared Caddyfile. Add a site
block equivalent to the generic example, replacing only the hostname:

```caddyfile
agentroom.example.invalid {
	@private path /api/v1/ingest /api/v1/ingest/* /api/v1/mcp /api/v1/mcp/* /api/v1/adapters /api/v1/adapters/*
	respond @private "Not Found" 404

	reverse_proxy 127.0.0.1:8443
}
```

The private adapter paths are denied at public ingress; approved adapters use
the separate loopback-only port 9091 through an operator-managed encrypted
tunnel. Validate before reload:

```sh
sudo deploy/caddy/verify-caddy.sh /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

No Agent Room firewall rule or upstream certificate is needed. The kernel
cannot route a loopback listener from another machine, and `doctor.sh` verifies
the actual listener address.

## Deploy and roll back

Copy the signed release triplet and public verification key to `host_agentroom`, then run:

```sh
sudo deploy/deploy.sh \
  --artifact <agentroom-version-linux-amd64.tar.gz> \
  --public-key <cosign-public-key.pem> \
  --public-url https://<public-host>
```

Deployment verifies the artifact, stops Agent Room to quiesce writes, takes a
coordinated pgBackRest database backup and immutable artifact snapshot, then
writes one atomic backup record. Only then does it install an immutable version
directory, run forward migrations, start the service, and smoke-test public and
private boundaries. An application failure restores the previous binary
pointer; migrations are deliberately expand-only and are not automatically
reversed. Artifact snapshots default to
`/var/backups/agentroom-artifacts`, retain 14 archives, contain an exact
path/size/SHA-256 manifest, and never include Hermes data.

Roll back to the recorded prior release or an explicit installed version:

```sh
sudo deploy/rollback.sh --public-url https://<public-host>
sudo deploy/rollback.sh --version <installed-version> --public-url https://<public-host>
```

Run `sudo deploy/doctor.sh` after host or application changes.

## Restore drills

Restores are intentionally isolated and never overwrite production:

```sh
sudo deploy/restore.sh \
  --record /var/lib/agentroom/deployments/<version>.backup.json \
  --target /var/lib/agentroom-restore/<drill-id> \
  --artifact-target /var/lib/agentroom-artifact-restore/<drill-id> \
  --confirm RESTORE_TO_ISOLATED_TARGET
```

The command verifies the artifact archive checksum and every restored blob
against its manifest, rejects links and unmanifested files, and restores both
data stores only to isolated targets. Validate the restored cluster,
application schema, and artifact downloads separately before any planned
recovery cutover.

## Development and security verification

`deploy/compose/compose.yaml` provides PostgreSQL and an optional application
profile for development and integration testing. It also runs a pinned Dex
OIDC provider so the real authorization-code/PKCE browser flow can be tested.
All ports are published on loopback and application credentials use Docker
secrets.

The optional `app` profile is supported on the target Ubuntu Linux host. It
uses host networking so the browser, a native development binary, and the
application container all validate the same loopback issuer URL. Agent Room
still binds each of its listeners to `127.0.0.1`; do not change those addresses
to wildcard binds. Docker Desktop host-network behavior is platform-dependent,
so use the native development binary when testing on a non-Linux workstation.

The local issuer is `http://127.0.0.1:5556/dex`; the client ID is
`agentroom-local`, the client secret is `agentroom-dex-local-only`, and the
dev/test login is `operator@agentroom.local` with password
`agentroom-local-only`. These are fixtures, not production credentials. Set
the development Agent Room redirect URL to
`http://127.0.0.1:58443/api/v1/auth/callback`.

For the optional application profile, the `database-url` secret must point to
the published loopback PostgreSQL port, the `oidc-client-secret` file must
contain `agentroom-dex-local-only`, and the session secret must contain at
least 32 random bytes. Start it only with a locally built, reviewed image:

```sh
AGENTROOM_IMAGE=agentroom:<local-version> \
  docker compose -f deploy/compose/compose.yaml --profile app up --wait
```

Run local fail-closed checks with:

```sh
tests/security/deployment-static.sh
tests/security/artifact-fail-closed.sh
tests/security/completeness-scan.sh
tests/security/source-authorization-contract.sh
tests/security/static-security.sh
```

Against an authenticated staging deployment, run
`tests/security/session-boundary-regression.sh` with a root-only curl cookie
jar and distinct authorized/unauthorized project IDs. Test private adapter
scope separately with `tests/security/mcp-scope-regression.sh`; its token file
must contain an `event:ingest`-only service token.
Use `tests/security/mcp-session-binding-regression.sh` with two root-only token
files to prove that a restricted token cannot inherit `task:read` through a
session initialized by a privileged token.

Active ZAP testing is gated by an explicit authorization flag and should target
an isolated staging deployment:

```sh
AGENTROOM_PEN_TEST_AUTHORIZED=YES \
AGENTROOM_PEN_TARGET=https://staging.example.invalid \
  tests/security/authorized-pen-regression.sh
```
