# Agent Room deployment

Agent Room is deployed independently from Hermes. The supported production
target is a dedicated Ubuntu Linux/amd64 host, PostgreSQL stores durable state,
and Caddy on `host_ingress` is the only public ingress. Caddy authenticates to
Agent Room with mTLS; the application firewall also restricts port 8443 to
`host_ingress`. These installation scripts target Ubuntu Linux/amd64 and must
not be run on a Darwin development workstation.

## Supported production shape

- Ubuntu Linux on Intel x86-64 hardware
- versioned releases under `/opt/agentroom/releases/<version>`
- atomic `/opt/agentroom/current` symlink
- systemd units with encrypted credentials and AppArmor confinement
- PostgreSQL 18 with pgBackRest backups
- Caddy TLS termination on `host_ingress`, mTLS to `host_agentroom:8443`
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
not create database, OIDC, or TLS credentials and does not start Agent Room.

On a prepared host where prerequisites are already managed separately, install
only the Agent Room host files:

```sh
sudo deploy/install-host.sh --apply
sudoedit /etc/agentroom/agentroom.conf
```

Replace every `.invalid` value in the example configuration. The parser accepts
only documented `AGENTROOM_KEY=value` entries and treats unknown or duplicate
keys as fatal. Also replace the TEST-NET value in
`AGENTROOM_TRUSTED_PROXY_CIDRS` with the exact address or narrow CIDR used by
`host_ingress` when it connects to `host_agentroom`. Forwarded client addresses are honored only
when both that network check and the mTLS client-certificate check succeed.

Provision systemd credentials without placing secrets in the environment or
configuration:

```sh
sudo install -d -m 0700 /etc/agentroom/credentials
sudo systemd-creds encrypt - /etc/agentroom/credentials/database-url.cred
sudo systemd-creds encrypt - /etc/agentroom/credentials/session-secret.cred
sudo systemd-creds encrypt - /etc/agentroom/credentials/oidc-client-secret.cred
sudo systemd-creds encrypt <host_agentroom-server-key.pem> /etc/agentroom/credentials/tls-key.cred
sudo install -m 0644 <host_agentroom-server-cert.pem> /etc/agentroom/credentials/tls-cert.pem
sudo install -m 0644 <upstream-client-ca.pem> /etc/agentroom/credentials/tls-client-ca.pem
```

Each `systemd-creds encrypt - ...` command reads its secret from standard input;
do not put secret values in shell history. The server certificate must cover
the exact upstream DNS name configured by Caddy. The client CA should issue
only ingress identities authorized to reach Agent Room.

Configure pgBackRest using `deploy/pgbackrest/pgbackrest.conf.example` and the
PostgreSQL archive settings in `deploy/pgbackrest/postgresql.conf.snippet`.
Initialize and validate the stanza before first deployment:

```sh
sudo -u postgres pgbackrest --stanza=agentroom stanza-create
sudo -u postgres pgbackrest --stanza=agentroom check
```

## Configure `host_ingress`

Copy `deploy/caddy/Caddyfile.example` into the existing Caddy configuration and
replace the public host, upstream DNS name, CA, client certificate, and key.
Never use `tls_insecure_skip_verify`. Validate before reload:

```sh
sudo deploy/caddy/verify-caddy.sh /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

Apply the nftables example only after replacing the TEST-NET address with the
literal address of `host_ingress`. Preserve the host's existing policy and include the
Agent Room chain from the owning ruleset. Validate from `host_agentroom`:

```sh
sudo deploy/firewall/verify-firewall.sh --ingress-ip <ingress-ip>
```

Verify both positive and negative mTLS paths from `host_ingress`:

```sh
deploy/caddy/verify-upstream-mtls.sh \
  --url https://<host_agentroom-upstream-name>:8443/healthz \
  --ca <upstream-ca.pem> \
  --client-cert <host_ingress-client.pem> \
  --client-key <host_ingress-client-key.pem>
```

Providing an independently issued certificate through `--untrusted-cert` and
`--untrusted-key` also verifies rejection of the wrong client identity.

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

Run `sudo deploy/doctor.sh --public-url https://<public-host>` after host or
certificate changes.

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
