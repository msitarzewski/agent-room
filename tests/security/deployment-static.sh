#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
cd "${repo_root}"

failed=0
if rg -n 'tls_insecure_skip_verify|InsecureSkipVerify[[:space:]]*:[[:space:]]*true' \
  deploy/caddy/Caddyfile.example deploy/systemd deploy/config deploy/compose; then
  failed=1
fi
if rg -n 'Environment=.*(SECRET|PASSWORD|DATABASE_URL|TLS_KEY)=' deploy/systemd; then
  failed=1
fi
for required in \
  NoNewPrivileges=true \
  ProtectSystem=strict \
  ProtectHome=true \
  PrivateTmp=true \
  PrivateDevices=true \
  RestrictSUIDSGID=true \
  MemoryDenyWriteExecute=true; do
  grep -q "^${required}$" deploy/systemd/agentroom.service || {
    printf 'Missing systemd hardening: %s\n' "${required}" >&2
    failed=1
  }
done
grep -q '^CapabilityBoundingSet=$' deploy/systemd/agentroom.service || failed=1
grep -q '^ConditionFileIsExecutable=' deploy/systemd/agentroom.service || failed=1
! grep -q '^ConditionPathIsExecutable=' deploy/systemd/agentroom.service || failed=1
grep -q '@private path' deploy/caddy/Caddyfile.example || failed=1
grep -q '/api/v1/ingest.* /api/v1/mcp.* /api/v1/adapters' deploy/caddy/Caddyfile.example || failed=1
grep -q 'respond @private "Not Found" 404' deploy/caddy/Caddyfile.example || failed=1
grep -q 'reverse_proxy 127\.0\.0\.1:8443' deploy/caddy/Caddyfile.example || failed=1
if rg -n 'tls_client_auth|tls_trust_pool|AGENTROOM_TLS_|reverse_proxy https://127\.0\.0\.1:8443' \
  deploy/caddy/Caddyfile.example deploy/config deploy/systemd deploy/compose deploy/configure-instance.sh; then
  failed=1
fi
grep -q '^AGENTROOM_HTTP_ADDR=127\.0\.0\.1:8443$' deploy/config/agentroom.conf.example || failed=1
grep -q '^AGENTROOM_ADAPTER_ADDR=127\.0\.0\.1:9091$' deploy/config/agentroom.conf.example || failed=1
grep -q '^AGENTROOM_TRUSTED_PROXY_CIDRS=127\.0\.0\.1/32$' deploy/config/agentroom.conf.example || failed=1
grep -q '^AGENTROOM_WEB_DIR=/opt/agentroom/current/web$' deploy/config/agentroom.conf.example || failed=1
grep -q '^ReadOnlyPaths=.* /opt/agentroom/current/web$' deploy/systemd/agentroom.service || failed=1
grep -q '^  /etc/agentroom/ r,$' deploy/apparmor/opt.agentroom.agentroomctl || failed=1
grep -q '^  /etc/agentroom/ r,$' deploy/apparmor/opt.agentroom.agentroomd || failed=1
grep -q '^LoadCredentialEncrypted=oidc-client-secret:' deploy/systemd/agentroom-migrate.service || failed=1
grep -q 'run_agentroomctl_transient "agentroom-migration-check-\$\$"' deploy/migrate.sh || failed=1
grep -q 'run_agentroomctl_transient "agentroom-doctor-\$\$"' deploy/doctor.sh || failed=1
grep -q '^AGENTROOM_OIDC_REDIRECT_URL=https://agentroom\.example\.invalid/api/v1/auth/callback$' deploy/config/agentroom.conf.example || failed=1
grep -q 'AGENTROOM_OIDC_REDIRECT_URL: http://127\.0\.0\.1:.*\/api/v1/auth/callback' deploy/compose/compose.yaml || failed=1
grep -q 'SOURCE_DATE_EPOCH is required for production release packaging' deploy/package-release.sh || failed=1
grep -q 'SOURCE_REVISION must be the exact 40-character Git commit' deploy/package-release.sh || failed=1
grep -q 'vcs.modified' deploy/package-release.sh || failed=1
grep -q '.agentroom-build.json' deploy/package-release.sh || failed=1
grep -q 'gzip -n' deploy/package-release.sh || failed=1
grep -q "wc -c <\"\${file_path}\"" deploy/package-release.sh || failed=1
! grep -q "stat -c '%s'" deploy/package-release.sh || failed=1
grep -q 'cosign sign-blob --new-bundle-format=false' deploy/package-release.sh || failed=1
grep -q -- '--use-signing-config=false' deploy/package-release.sh || failed=1
grep -q 'cosign verify-blob --new-bundle-format=false' deploy/verify-artifact.sh || failed=1
grep -q 'Required license material is missing' deploy/package-release.sh || failed=1
grep -q 'THIRD_PARTY_NOTICES.md' deploy/package-release.sh || failed=1
grep -q 'Release archive is missing third-party license texts' tests/security/reproducible-release.sh || failed=1
grep -q 'tests/security/license-compliance.sh' .github/workflows/security.yml || failed=1
grep -q 'github.com/google/go-licenses@v1.6.0' .github/workflows/security.yml || failed=1
grep -q 'HOME: /root' .github/workflows/security.yml || failed=1
grep -q 'PLAYWRIGHT_BROWSERS_PATH: /ms-playwright' .github/workflows/security.yml || failed=1
grep -q 'pipx install semgrep==1.124.0' .github/workflows/security.yml || failed=1
grep -q 'pipx inject --force semgrep setuptools==80.9.0' .github/workflows/security.yml || failed=1
grep -q '^  RIPGREP_VERSION: "14\.1\.1"$' .github/workflows/security.yml || failed=1
grep -q '^  RIPGREP_AMD64_SHA256: "2f0c732ef166b4f7be7190d4012d60b3f8467bdd6f795c0598817bd2ac1706ae"$' \
  .github/workflows/security.yml || failed=1
[[ "$(grep -c 'name: Install pinned Ripgrep' .github/workflows/security.yml)" -eq 2 ]] || failed=1
grep -q 'dpkg-deb --extract' .github/workflows/security.yml || failed=1
grep -q "dex_oidc_subject='CiQyMzhiNmY3Yi0xN2JkLTRiMGQtYTE5NS0yNmU3MjViNzc2Y2ESBWxvY2Fs'" \
  tests/security/real-daemon-browser.sh || failed=1
grep -q 'coverage + 0 >= 80' tests/security/backend-quality.sh || failed=1
grep -q 'coverage + 0 >= 90' tests/security/backend-quality.sh || failed=1
grep -q 'tests/security/backend-quality.sh' .github/workflows/security.yml || failed=1
grep -q 'tests/security/backend-quality.sh' deploy/build-release.sh || failed=1
grep -q 'npm run api:lint -- --extends recommended-strict' .github/workflows/security.yml || failed=1
grep -q -- '-exclude-dir=internal/postgres/sqlcgen' .github/workflows/security.yml || failed=1
grep -q -- '-exclude=G124,G204' .github/workflows/security.yml || failed=1
grep -q -- '-exclude=G124,G204' tests/security/static-security.sh || failed=1
grep -q -- '-coverpkg=./internal/...' tests/security/backend-quality.sh || failed=1
grep -q './internal/... ./tests/security' tests/security/backend-quality.sh || failed=1
grep -q './internal/postgres' tests/security/backend-quality.sh || failed=1
grep -q './tests/api' tests/security/backend-quality.sh || failed=1
grep -q 'npm --prefix web run api:lint -- --extends recommended-strict' deploy/build-release.sh || failed=1
[[ ! -e .redocly.lint-ignore.yaml && ! -e web/.redocly.lint-ignore.yaml ]] || failed=1
grep -q 'confirm-quiesced AGENTROOM_STOPPED' deploy/deploy.sh || failed=1
grep -q '^release_installed=0$' deploy/deploy.sh || failed=1
grep -q '^release_installed=1$' deploy/deploy.sh || failed=1
grep -q 'rm -rf -- "${release_dir}"' deploy/deploy.sh || failed=1
grep -q 'artifact-backup.sh' deploy/backup.sh || failed=1
grep -q 'artifact-restore.sh' deploy/restore.sh || failed=1
grep -q "id = \"GO-2026-5932\"" .github/workflows/security.yml || failed=1
grep -q "ignoreUntil = 2026-10-27" .github/workflows/security.yml || failed=1
grep -q 'go_dependencies="$(go list -deps ./...)"' .github/workflows/security.yml || failed=1
grep -q "grep -q '\\^golang.org/x/crypto/openpgp'" .github/workflows/security.yml || failed=1
grep -q "id = \"GO-2026-5932\"" tests/security/static-security.sh || failed=1
grep -q 'honnef.co/go/tools/cmd/staticcheck@v0.7.0' .github/workflows/security.yml || failed=1
grep -q 'staticcheck -tags=integration' .github/workflows/security.yml || failed=1
grep -q 'staticcheck -tags=integration' tests/security/static-security.sh || failed=1
grep -q 'deny /home/\* /\\.hermes' deploy/apparmor/opt.agentroom.agentroomd 2>/dev/null && failed=1
grep -Eq 'image: postgres@sha256:[0-9a-f]{64} # 18\.4-bookworm$' deploy/compose/compose.yaml || failed=1
grep -Eq 'image: ghcr\.io/dexidp/dex@sha256:[0-9a-f]{64} # v2\.45\.1-alpine$' deploy/compose/compose.yaml || failed=1
grep -Eq 'image: postgres@sha256:[0-9a-f]{64} # 18\.4-bookworm$' .github/workflows/security.yml || failed=1
grep -q '^    network_mode: host$' deploy/compose/compose.yaml || failed=1
grep -q 'AGENTROOM_OIDC_ISSUER: http://127\.0\.0\.1:' deploy/compose/compose.yaml || failed=1
grep -q "^readonly POSTGRESQL_MAJOR=18$" deploy/install-host.sh || failed=1
grep -q "^readonly POSTGRESQL_APT_KEY_FINGERPRINT='B97B0AFCAA1A47F044F244A07FCC7D46ACCC4CF8'$" \
  deploy/install-host.sh || failed=1
grep -q "^readonly COSIGN_VERSION='3\\.0\\.6'$" deploy/install-host.sh || failed=1
grep -q "^readonly COSIGN_AMD64_SHA256='e16e8eb815f8b1b3cee3e678874393c286f19dd59e9ac5da95e428f970ef00f3'$" \
  deploy/install-host.sh || failed=1
grep -q "dpkg --print-architecture.*amd64" deploy/install-host.sh || failed=1
grep -q "shared_buffers = '256MB'" deploy/install-host.sh || failed=1
grep -q '^max_connections = 50$' deploy/install-host.sh || failed=1
grep -q 'sha256sum --check --strict' deploy/install-host.sh || failed=1
grep -q 'HTTP listener is loopback-only' deploy/doctor.sh || failed=1
! grep -q 'require_command nft' deploy/doctor.sh || failed=1
grep -q 'AGENTROOM_HTTP_ADDR=127.0.0.1:8443' deploy/configure-instance.sh || failed=1
grep -q 'systemd-creds encrypt --name=' deploy/configure-instance.sh || failed=1
! grep -q 'PKI\\|tls-key\\|Caddyfile.site' deploy/configure-instance.sh || failed=1
grep -q 'pgbackrest --stanza=agentroom check' deploy/configure-instance.sh || failed=1

install_help="$(deploy/install-host.sh --help 2>&1)" || failed=1
grep -q 'install-host.sh --bootstrap --apply' <<<"${install_help}" || failed=1
if deploy/install-host.sh --not-a-real-option >/dev/null 2>&1; then
  printf 'install-host.sh accepted an unknown option\n' >&2
  failed=1
fi

configure_help="$(deploy/configure-instance.sh --help 2>&1)" || failed=1
grep -q 'configure-instance.sh' <<<"${configure_help}" || failed=1
if deploy/configure-instance.sh --not-a-real-option >/dev/null 2>&1; then
  printf 'configure-instance.sh accepted an unknown option\n' >&2
  failed=1
fi

while IFS= read -r action_use; do
  if [[ ! "${action_use}" =~ uses:[[:space:]]+[^@[:space:]]+@[0-9a-f]{40}[[:space:]]+\#[[:space:]]+v[0-9] ]]; then
    printf 'GitHub Action is not pinned to an immutable SHA with a version comment: %s\n' "${action_use}" >&2
    failed=1
  fi
done < <(rg '^[[:space:]]*uses:' .github/workflows)

for script in deploy/*.sh deploy/caddy/*.sh tests/security/*.sh; do
  bash -n "${script}"
done

((failed == 0)) || { printf 'Deployment static checks failed\n' >&2; exit 1; }
printf 'Deployment static checks passed\n'
