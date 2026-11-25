#!/usr/bin/env bash
# Michael-run shared-host migration. Never run the dedicated-host installers here.
# Reuses verify-artifact.sh; only this site's Supervisor groups and Serve ports apply.
set -Eeuo pipefail
IFS=$'\n\t'
umask 0077

SITE=/srv/www/agentroom
RUNTIME=/var/lib/agentroom-shared/github
SUPERVISOR=/etc/supervisor/conf.d/agentroom-host.conf
PROGRAMS=(agentroom-web agentroom-github)

fail() { printf 'Shared-host migration: %s\n' "$*" >&2; exit 1; }
note() { printf '%s\n' "$*"; }

# Kept callable when sourced so path/inventory tests never need sudo or a host.
validate_bundle() {
  python3 - "$1" <<'PY'
import configparser, hashlib, json, os, pathlib, re, shlex, stat, sys, tomllib, urllib.parse
def need(ok, message):
    if not ok: raise ValueError(message)
def pairs(items):
    result = {}
    for key, value in items:
        need(key not in result, 'duplicate JSON key')
        result[key] = value
    return result
def read(path): return json.loads(path.read_text(), object_pairs_hook=pairs)
def ident(value): return isinstance(value, str) and re.fullmatch(r'[a-z_][a-z0-9_]{0,62}', value)
try:
    raw = sys.argv[1]
    root = pathlib.Path(raw)
    need(root.is_absolute() and raw == str(root) and '..' not in root.parts and root != pathlib.Path('/'), 'bundle must be a normalized absolute directory')
    for path in [root, *root.parents]: need(not path.is_symlink(), 'bundle ancestors may not be symlinks')
    need(root.is_dir(), 'bundle directory absent')
    inventory = {}
    for path in [root, *root.rglob('*')]:
        info = path.lstat()
        need(stat.S_ISREG(info.st_mode) or stat.S_ISDIR(info.st_mode), 'bundle contains a link or special file')
        need(info.st_uid == os.getuid() and not info.st_mode & 0o022, 'bundle must be owned by operator and not writable by others')
        if path == root: continue
        rel = path.relative_to(root).as_posix()
        need(re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9._/+@-]*', rel) and all(p not in ('.', '..') for p in rel.split('/')), 'unsafe bundle path')
        if path.is_file():
            need(info.st_nlink == 1, 'bundle hard links forbidden')
            if rel != 'manifest.json': inventory[rel] = (info.st_size, hashlib.sha256(path.read_bytes()).hexdigest())
    manifest = read(root / 'manifest.json')
    need(manifest.get('schema') == 1 and isinstance(manifest.get('files'), list), 'invalid bundle manifest')
    listed = {}
    for entry in manifest['files']:
        path, size, digest = entry.get('path'), entry.get('size'), entry.get('sha256')
        need(isinstance(path, str) and path not in listed and type(size) is int and size >= 0 and isinstance(digest, str) and re.fullmatch('[a-f0-9]{64}', digest), 'invalid manifest entry')
        listed[path] = (size, digest)
    need(inventory == listed, 'bundle file inventory, size or digest mismatch')
    for rel in ['release/bin/agentroomd', 'release/bin/agentroomctl', 'release/web/index.html', 'release/manifest.json', 'tools/cosign', 'verify-artifact.sh', 'payload/release.tar.gz', 'payload/release.tar.gz.sha256', 'payload/release.tar.gz.sig', 'payload/release-public.pem', 'compose.json', 'agentroom.conf', 'dex-config.json', 'supervisor.conf', 'github-run.sh', 'github/connector.toml', 'github/source-token', 'github/state/queue.sqlite3', 'adapters/source.py', 'adapters/github/agentroom_github.py', 'restore.sql', 'dumps/app.dump', 'dumps/identity.dump', 'secrets/database-url', 'secrets/session-secret', 'secrets/oidc-client-secret', 'secrets/postgres-admin-password', 'snapshot.json']:
        need(rel in inventory and inventory[rel][0] > 0, 'required bundle input absent')
    meta = read(root / 'metadata.json')
    need(meta.get('schema') == 1 and meta.get('site') == 'agentroom' and meta.get('svcuser') == 'site_agentroom', 'incorrect site identity')
    snapshot = read(root / 'snapshot.json')
    need(snapshot.get('schema') == 1 and snapshot.get('kind') in ('rehearsal', 'final') and type(snapshot.get('source_fenced')) is bool and isinstance(snapshot.get('created_at'), str), 'invalid snapshot phase')
    need(snapshot['kind'] != 'final' or snapshot['source_fenced'] is True, 'final snapshot requires fenced source')
    need(re.fullmatch('[a-f0-9]{40}', meta.get('source_revision', '')), 'invalid release revision')
    ports = meta.get('ports', {})
    need(set(ports) == {'http', 'oidc', 'postgres', 'admin', 'adapter'} and all(type(v) is int and 1024 <= v <= 65535 for v in ports.values()) and len(set(ports.values())) == 5 and ports['http'] == 4445 and ports['oidc'] == 5557, 'invalid or overlapping private ports')
    for key in ['app_database', 'identity_database', 'app_role', 'identity_role', 'postgres_admin']: need(ident(meta.get(key)), 'unsafe database identity')
    need(meta['app_database'] != meta['identity_database'] and meta['app_role'] != meta['identity_role'] and not {'postgres', 'template0', 'template1'} & {meta['app_database'], meta['identity_database']}, 'databases/roles must be separate application identities')
    public = urllib.parse.urlsplit(meta['public_url']); issuer = urllib.parse.urlsplit(meta['issuer'])
    need(public.scheme == 'https' and public.hostname and public.hostname.endswith('.ts.net') and public.port == 4445 and public.path == '' and not public.query and not public.fragment and not public.username and not public.password, 'public URL must be private tailnet origin on 4445')
    need(meta['issuer'] == 'https://' + public.hostname + ':4446/dex', 'issuer must use same private tailnet host on 4446')
    site = '/srv/www/agentroom'; runtime = '/var/lib/agentroom-shared/github'
    cfg = {}
    for line in (root / 'agentroom.conf').read_text().splitlines():
        if not line.strip() or line.lstrip().startswith('#'): continue
        key, sep, value = line.partition('=')
        need(sep and re.fullmatch('AGENTROOM_[A-Z_]+', key) and key not in cfg, 'invalid app configuration')
        words = shlex.split(value); need(len(words) <= 1, 'invalid app configuration literal')
        cfg[key] = words[0] if words else ''
    expected = {'AGENTROOM_HTTP_ADDR': f"127.0.0.1:{ports['http']}", 'AGENTROOM_ADMIN_ADDR': f"127.0.0.1:{ports['admin']}", 'AGENTROOM_ADAPTER_ADDR': f"127.0.0.1:{ports['adapter']}", 'AGENTROOM_PUBLIC_URL': meta['public_url'], 'AGENTROOM_ALLOWED_ORIGINS': meta['public_url'], 'AGENTROOM_OIDC_ISSUER': meta['issuer'], 'AGENTROOM_OIDC_REDIRECT_URL': meta['public_url'] + '/api/v1/auth/callback', 'AGENTROOM_DEV': 'false', 'AGENTROOM_TRUSTED_PROXY_CIDRS': '127.0.0.1/32', 'AGENTROOM_ARTIFACT_DIR': site + '/storage/artifacts', 'AGENTROOM_WEB_DIR': site + '/current/web', 'AGENTROOM_WORKSPACE_ROOT': site + '/storage/workspaces'}
    for key, value in expected.items(): need(cfg.get(key) == value, 'app private boundary/configuration mismatch')
    for key, name in [('AGENTROOM_DATABASE_URL_FILE', 'database-url'), ('AGENTROOM_SESSION_SECRET_FILE', 'session-secret'), ('AGENTROOM_OIDC_CLIENT_SECRET_FILE', 'oidc-client-secret')]: need(cfg.get(key) == site + '/secrets/' + name, 'secret path escapes site')
    need(not cfg.get('AGENTROOM_CODEX_BIN') and not cfg.get('AGENTROOM_CLAUDE_BIN') and not cfg.get('AGENTROOM_OTEL_EXPORTER_OTLP_ENDPOINT'), 'migration may not enable execution or external telemetry')
    db = urllib.parse.urlsplit((root / 'secrets/database-url').read_text().strip())
    need(db.scheme in ('postgres', 'postgresql') and db.hostname == '127.0.0.1' and db.port == ports['postgres'] and urllib.parse.unquote(db.username or '') == meta['app_role'] and db.path == '/' + meta['app_database'], 'app database target mismatch')
    compose = read(root / 'compose.json')
    need(set(compose) == {'name', 'services', 'volumes', 'secrets'} and compose['name'] == 'agentroom-host' and set(compose['services']) == {'postgres', 'dex'}, 'compose scope must contain only own PostgreSQL and Dex')
    need(compose['volumes'] == {'postgres-data': {'name': 'agentroom-host-postgres'}} and compose['secrets'] == {'postgres-password': {'file': './secrets/postgres-admin-password'}}, 'compose storage escapes dedicated namespace')
    pg, dex = compose['services']['postgres'], compose['services']['dex']
    need(set(pg) <= {'image', 'restart', 'environment', 'secrets', 'ports', 'volumes', 'healthcheck', 'security_opt'} and set(dex) <= {'image', 'restart', 'command', 'ports', 'volumes', 'security_opt', 'cap_drop', 'read_only', 'tmpfs'}, 'unexpected compose privileges or host bindings')
    need(re.fullmatch(r'postgres@sha256:[a-f0-9]{64}', pg.get('image', '')) and re.fullmatch(r'ghcr.io/dexidp/dex@sha256:[a-f0-9]{64}', dex.get('image', '')), 'images must use expected registries and immutable digests')
    need(pg.get('environment') == {'POSTGRES_USER': meta['postgres_admin'], 'POSTGRES_DB': meta['app_database'], 'POSTGRES_PASSWORD_FILE': '/run/secrets/postgres-password'} and pg.get('secrets') == ['postgres-password'] and pg.get('ports') == [f"127.0.0.1:{ports['postgres']}:5432"] and pg.get('volumes') == ['postgres-data:/var/lib/postgresql'], 'PostgreSQL boundary mismatch')
    need(pg.get('security_opt') == ['no-new-privileges:true'] and pg.get('healthcheck') == {'test': ['CMD-SHELL', 'pg_isready -U ' + meta['postgres_admin']], 'interval': '2s', 'timeout': '3s', 'retries': 30}, 'unsafe PostgreSQL health command or security policy')
    need(dex.get('ports') == ['127.0.0.1:5557:5556'] and dex.get('volumes') == ['./dex-config.json:/etc/dex/config.json:ro'] and dex.get('command') == ['dex', 'serve', '/etc/dex/config.json'] and dex.get('read_only') is True and dex.get('cap_drop') == ['ALL'] and dex.get('security_opt') == ['no-new-privileges:true'] and dex.get('tmpfs') == ['/tmp:rw,noexec,nosuid,nodev,size=32m'], 'Dex boundary mismatch')
    identity = read(root / 'dex-config.json')
    need(identity.get('issuer') == meta['issuer'] and identity.get('web') == {'http': '0.0.0.0:5556'} and identity.get('storage', {}).get('type') == 'postgres', 'identity endpoint mismatch')
    store = identity['storage']['config']
    need(store.get('host') == 'postgres' and store.get('port') == 5432 and store.get('database') == meta['identity_database'] and store.get('user') == meta['identity_role'] and store.get('ssl') == {'mode': 'disable'}, 'identity database target mismatch')
    clients = identity.get('staticClients', [])
    need(len(clients) == 1 and clients[0].get('id') == cfg.get('AGENTROOM_OIDC_CLIENT_ID') and clients[0].get('redirectURIs') == [meta['public_url'] + '/api/v1/auth/callback'] and clients[0].get('secret') == (root / 'secrets/oidc-client-secret').read_text().strip(), 'identity client mismatch')
    need(len(identity.get('staticPasswords', [])) == 1 and identity['staticPasswords'][0].get('userID') and identity['staticPasswords'][0].get('hash', '').startswith(('$2a$', '$2b$', '$2y$')), 'preserved identity subject/bcrypt required')
    connector = tomllib.loads((root / 'github/connector.toml').read_text())
    need(connector['connector'].get('state_dir') == runtime + '/state' and connector['connector'].get('ingest_url') == f"http://127.0.0.1:{ports['adapter']}/api/v1/ingest", 'collector private target mismatch')
    need(not connector['connector'].get('github_token_file') or connector['connector']['github_token_file'].startswith(runtime + '/'), 'GitHub API secret must remain in private runtime')
    need(connector.get('repositories') and all(row.get('token_file') == runtime + '/source-token' for row in connector['repositories']), 'source token escapes private runtime')
    expected_runner = '#!/bin/sh\nset -eu\nexec /usr/bin/python3 -B ' + site + '/adapters/github/agentroom_github.py --config ' + runtime + '/connector.toml watch\n'
    need((root / 'github-run.sh').read_text() == expected_runner, 'unexpected collector command')
    parser = configparser.ConfigParser(interpolation=None, strict=True); parser.read(root / 'supervisor.conf')
    commands = {'web': site + '/current/bin/agentroomd --config ' + site + '/agentroom.conf', 'github': site + '/github-run.sh'}
    need(set(parser.sections()) == {'program:agentroom-web', 'program:agentroom-github'} and not parser.defaults(), 'Supervisor configuration may contain only own programs')
    for name, command in commands.items():
        section = parser['program:agentroom-' + name]
        expected = {'command': command, 'directory': site, 'user': 'site_agentroom', 'autostart': 'false', 'autorestart': 'true', 'startsecs': '3', 'startretries': '60', 'stopsignal': 'TERM', 'stopasgroup': 'true', 'killasgroup': 'true', 'umask': '0077', 'redirect_stderr': 'true', 'stdout_logfile': site + '/storage/logs/' + name + '.log', 'stdout_logfile_maxbytes': '10MB', 'stdout_logfile_backups': '3'}
        need(dict(section) == expected, 'unexpected Supervisor command, identity or policy')
    print(hashlib.sha256((root / 'manifest.json').read_bytes()).hexdigest())
except (OSError, ValueError, KeyError, TypeError, configparser.Error) as exc:
    print('Bundle validation failed: ' + str(exc), file=sys.stderr); sys.exit(1)
PY
}

verify_release() {
  local scratch
  scratch="$(mktemp -d "${BUNDLE}/.verify-XXXXXXXX")"
  # Both verifier scratch lists and its extraction stay within the private bundle.
  mkdir "${scratch}/release"
  if ! PATH="${BUNDLE}/tools:${PATH}" TMPDIR="${scratch}" bash "${BUNDLE}/verify-artifact.sh" \
    --artifact "${BUNDLE}/payload/release.tar.gz" --public-key "${BUNDLE}/payload/release-public.pem" \
    --extract-to "${scratch}/release" >/dev/null; then
    rm -rf -- "${scratch}"; fail 'signed release verification failed'
  fi
  if ! diff -qr -- "${scratch}/release" "${BUNDLE}/release" >/dev/null; then
    rm -rf -- "${scratch}"; fail 'installed release differs from signed payload'
  fi
  [[ "$(jq -r '.source_revision' "${scratch}/release/manifest.json")" == "$REVISION" ]] || {
    rm -rf -- "${scratch}"; fail 'release provenance mismatch';
  }
  rm -rf -- "${scratch}"
  [[ "$(validate_bundle "$BUNDLE")" == "$BUNDLE_DIGEST" ]] || fail 'bundle changed during verification'
}

safe_host_paths() {
  sudo python3 - "$SITE" "$RUNTIME" "$SUPERVISOR" <<'PY'
import os, pathlib, stat, sys
for raw in sys.argv[1:]:
    path = pathlib.Path(raw)
    for parent in [path, *path.parents]:
        if parent.is_symlink(): raise SystemExit('Refusing host symlink at deployment boundary')
    if path.exists() and path.is_dir():
        for child in path.rglob('*'):
            # The one intentional release selector is checked separately below.
            if child == pathlib.Path('/srv/www/agentroom/current') and child.is_symlink(): continue
            info = child.lstat()
            if not (stat.S_ISDIR(info.st_mode) or stat.S_ISREG(info.st_mode)) or (stat.S_ISREG(info.st_mode) and info.st_nlink != 1):
                raise SystemExit('Refusing host links or special files in site/runtime')
PY
  if [[ -L "$SITE/current" ]]; then
    [[ "$(readlink "$SITE/current")" =~ ^releases/[a-f0-9]{40}$ ]] || fail 'unsafe current release selector'
  fi
}

validate_operator_files() {
  python3 - "$HOME" <<'PY'
import os, pathlib, re, stat, sys
home = pathlib.Path(sys.argv[1])
for name in ['.agentroom-shared-host.lock', '.agentroom-shared-host-intent']:
    path = home / name
    if not path.exists() and not path.is_symlink(): continue
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_nlink != 1 or info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise SystemExit('Unsafe operator lock/intent file; refusing to follow or truncate it')
    if name.endswith('lock') and info.st_size != 0: raise SystemExit('Unexpected operator lock content')
    if name.endswith('intent') and not re.fullmatch('[a-f0-9]{64}\n', path.read_text()): raise SystemExit('Unexpected provisioning intent content')
PY
}

compose() { docker compose --project-name agentroom-host --file "$SITE/compose.json" "$@"; }
marker() { [[ -f "$SITE/.shared-host-$1" ]] && [[ "$(cat "$SITE/.shared-host-$1")" == "${2:-$BUNDLE_DIGEST}" ]]; }
write_marker() { printf '%s\n' "$BUNDLE_DIGEST" > "$SITE/.shared-host-$1"; chmod 0640 "$SITE/.shared-host-$1"; }
provision_intent() {
  [[ -f "$HOME/.agentroom-shared-host-intent" && ! -L "$HOME/.agentroom-shared-host-intent" ]] &&
    [[ "$(cat "$HOME/.agentroom-shared-host-intent")" == "${1:-$BUNDLE_DIGEST}" ]]
}

require_hardener() {
  [[ -f "$HOME/harden-site.sh" && ! -L "$HOME/harden-site.sh" && -r "$HOME/harden-site.sh" ]] || fail 'canonical ~/harden-site.sh is required'
}

supervisor_state() {
  local response
  response="$(sudo supervisorctl status "$1" 2>/dev/null || true)"
  if [[ "$response" == *'no such process'* || -z "$response" ]]; then printf 'ABSENT\n'; return; fi
  awk 'NR==1 {print $2}' <<<"$response"
}
require_stopped() {
  local program state
  for program in "${PROGRAMS[@]}"; do
    state="$(supervisor_state "$program")"
    case "$state" in ABSENT|STOPPED|EXITED|FATAL) ;; *) fail 'own Supervisor programs must be stopped before provisioning/restore' ;; esac
  done
  if id site_agentroom >/dev/null 2>&1; then
    ! pgrep -u site_agentroom >/dev/null || fail 'runtime account still has processes; stop only its own programs first'
  fi
}

# Reject an occupied route even if it points at our destination, unless our marker
# proves we created it. No reset, Funnel, or unrelated Serve update is ever used.
serve_state() {
  local port="$1" target="$2" owned=false
  [[ -f "$SITE/.shared-host-serve-$port" ]] && [[ "$(cat "$SITE/.shared-host-serve-$port")" == "$PUBLIC_URL" ]] && owned=true
  tailscale serve status --json | python3 -c '
import json, sys
config=json.load(sys.stdin); port=sys.argv[1]; target=sys.argv[2]; host=sys.argv[3]; owned=sys.argv[4]=="true"
key=host+":"+port
def mentions(value):
    if isinstance(value,dict): return any((k==port or k.endswith(":"+port)) or mentions(v) for k,v in value.items())
    if isinstance(value,list): return any(mentions(v) for v in value)
    return False
tcp=config.get("TCP",{}).get(port); web=config.get("Web",{}).get(key)
other={k:v for k,v in config.items() if k not in ("TCP","Web","AllowFunnel")}
funnel=config.get("AllowFunnel",{})
if any(v and k.endswith(":"+port) for k,v in funnel.items()): raise SystemExit("Refusing public Funnel on own port")
web_matches=[k for k in config.get("Web",{}) if k.endswith(":"+port)]
if tcp is None and not web_matches and not mentions(other): print("absent")
elif owned and tcp=={"HTTPS":True} and web=={"Handlers":{"/":{"Proxy":target}}} and web_matches==[key] and not mentions(other): print("owned")
else: raise SystemExit("Refusing existing or ambiguous Serve mapping")
' "$port" "$target" "$TAILNET_HOST" "$owned"
}

check_cluster_scope() {
  local value labels
  local -a ids=()
  mapfile -t ids < <(docker ps -aq --filter label=com.docker.compose.project=agentroom-host)
  if ((${#ids[@]})); then
    [[ -f "$SITE/.shared-host-provisioned" ]] || fail 'existing containers lack own provisioning marker'
    labels="$(docker inspect --format '{{ index .Config.Labels "com.docker.compose.service" }}' "${ids[@]}")"
    while IFS= read -r value; do
      [[ "$value" == postgres || "$value" == dex ]] || fail 'unexpected container in own Compose namespace'
    done <<< "$labels"
  fi
  if docker volume inspect agentroom-host-postgres >/dev/null 2>&1; then
    [[ -f "$SITE/.shared-host-provisioned" ]] || fail 'existing volume lacks own provisioning marker'
    [[ "$(docker volume inspect --format '{{ index .Labels "com.docker.compose.project" }}:{{ index .Labels "com.docker.compose.volume" }}' agentroom-host-postgres)" == 'agentroom-host:postgres-data' ]] || fail 'volume belongs to another deployment'
  fi
}

check_ports() {
  # Existing listeners are allowed only for this exact deployment's running
  # processes/containers, or tailscaled for a route whose marker was verified.
  local listeners native_pid containers
  native_pid="$(sudo supervisorctl pid agentroom-web 2>/dev/null || true)"
  containers="$(docker ps -q --filter label=com.docker.compose.project=agentroom-host)"
  local -a ids=()
  if [[ -n "$containers" ]]; then mapfile -t ids <<< "$containers"; fi
  local bindings='[]'
  if ((${#ids[@]})); then bindings="$(docker inspect "${ids[@]}")"; fi
  listeners="$(sudo ss -H -ltnp)"
  BINDINGS="$bindings" LISTENERS="$listeners" python3 - "$HTTP_PORT" "$OIDC_PORT" "$PG_PORT" "$ADMIN_PORT" "$ADAPTER_PORT" "$native_pid" <<'PY'
import json, os, re, sys
http, oidc, pg, admin, adapter = map(int, sys.argv[1:6]); native = sys.argv[6]
allowed = {http, oidc, pg, admin, adapter, 4446}
bindings = {}
for container in json.loads(os.environ['BINDINGS']):
    service = container['Config'].get('Labels', {}).get('com.docker.compose.service')
    expected = ('5432/tcp', pg) if service == 'postgres' else ('5556/tcp', oidc) if service == 'dex' else None
    if expected is None: raise SystemExit('Unexpected container in site namespace')
    for private, values in container.get('HostConfig', {}).get('PortBindings', {}).items():
        if private != expected[0] or values != [{'HostIp': '127.0.0.1', 'HostPort': str(expected[1])}]: raise SystemExit('Container port binding escaped own loopback boundary')
        bindings[expected[1]] = service
for row in os.environ['LISTENERS'].splitlines():
    fields = row.split()
    if len(fields) < 4: raise SystemExit('Unrecognized listener inventory')
    local = fields[3]; match = re.search(r':(\d+)$', local)
    if not match or int(match[1]) not in allowed: continue
    port = int(match[1]); pids = re.findall(r'pid=(\d+)', row)
    if '"tailscaled"' in row and port in (http, 4446): continue
    if local != '127.0.0.1:' + str(port): raise SystemExit('Own listener is not confined to loopback')
    if port in (http, admin, adapter) and native.isdigit() and int(native) > 0 and pids == [native]: continue
    if port in bindings and '"docker-proxy"' in row: continue
    raise SystemExit('Proposed port is occupied by an unrelated listener')
PY
}

preflight() {
  [[ "$(uname -s)" == Linux && "$(uname -m)" == x86_64 ]] || fail 'destination must be Linux/amd64'
  [[ "$(id -un)" == michael && "$(id -u)" != 0 ]] || fail 'run as michael; sudo is used only for scoped provisioning/service actions'
  local command_name port state
  for command_name in python3 jq docker sudo supervisorctl tailscale curl ss sha256sum file tar diff pgrep getent flock; do
    command -v "$command_name" >/dev/null || fail "required command absent: $command_name"
  done
  python3 -c 'import sys; assert sys.version_info >= (3,11)' || fail 'collector requires Python 3.11 or later'
  docker compose version >/dev/null
  TAILNET_HOST="$(tailscale status --json | jq -er '.Self.DNSName | sub("\\.$"; "")')"
  [[ "https://${TAILNET_HOST}:4445" == "$PUBLIC_URL" ]] || fail 'bundle public hostname differs from destination tailnet node'
  sudo -v
  safe_host_paths
  check_cluster_scope
  for port in 4445 4446; do
    if [[ "$port" == 4445 ]]; then state="$(serve_state "$port" http://127.0.0.1:4445)"; else state="$(serve_state "$port" http://127.0.0.1:5557)"; fi
    [[ "$state" == absent || "$state" == owned ]] || fail 'invalid Serve state'
  done
  check_ports
  if [[ ! -f "$SITE/.shared-host-provisioned" ]]; then
    for program in "${PROGRAMS[@]}"; do
      [[ "$(supervisor_state "$program")" == ABSENT ]] || fail 'Supervisor program name is already claimed by another configuration'
    done
    for port in "$HTTP_PORT" "$OIDC_PORT" "$PG_PORT" "$ADMIN_PORT" "$ADAPTER_PORT" 4446; do
      [[ -z "$(ss -H -ltn "sport = :$port")" ]] || fail 'proposed listener port is already occupied'
    done
    [[ ! -e "$SUPERVISOR" ]] || fail 'existing Supervisor config lacks own provisioning marker'
    if ! provision_intent; then
      [[ ! -d "$SITE" && ! -d "$RUNTIME" ]] || fail 'existing site/runtime lacks own provisioning marker'
      ! id site_agentroom >/dev/null 2>&1 || fail 'runtime account already exists without own provisioning marker'
      ! getent group site_agentroom >/dev/null || fail 'site group already exists without own provisioning marker'
    fi
  fi
}

install_release() {
  local release_digest
  release_digest="$(sha256sum "$BUNDLE/release/manifest.json" | awk '{print $1}')"
  if [[ -f "$SITE/.shared-host-release-$REVISION" ]]; then
    [[ "$(cat "$SITE/.shared-host-release-$REVISION")" == "$release_digest" ]] || fail 'completed release provenance changed'
    diff -qr "$BUNDLE/release" "$SITE/releases/$REVISION" >/dev/null || fail 'immutable installed release differs'
  else
    provision_intent || fail 'incomplete release requires exact provisioning intent'
    # A copy may have been interrupted after its manifest arrived. Until this
    # completion marker exists, only this fenced, exact-bundle install resumes it.
    cp -a "$BUNDLE/release/." "$SITE/releases/$REVISION/"
    diff -qr "$BUNDLE/release" "$SITE/releases/$REVISION" >/dev/null || fail 'release copy incomplete'
    printf '%s\n' "$release_digest" > "$SITE/.shared-host-release-$REVISION"
    chmod 0640 "$SITE/.shared-host-release-$REVISION"
  fi
}

provision() {
  if [[ -f "$SITE/.shared-host-activated" ]]; then
    marker activated || fail 'active deployment may not be overwritten by another bundle'
    note 'Already activated; provisioning makes no changes.'; return
  fi
  require_stopped
  require_hardener
  # Persist ownership intent before group/user/tree changes, permitting an exact
  # bundle to resume after interruption while rejecting unrelated existing sites.
  printf '%s\n' "$BUNDLE_DIGEST" > "$HOME/.agentroom-shared-host-intent"
  chmod 0600 "$HOME/.agentroom-shared-host-intent"
  if ! getent group site_agentroom >/dev/null; then sudo groupadd --system site_agentroom; fi
  if ! id site_agentroom >/dev/null 2>&1; then sudo useradd --system --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin --gid site_agentroom site_agentroom; fi
  [[ "$(id -gn site_agentroom)" == site_agentroom ]] || fail 'runtime primary group mismatch'
  case "$(getent passwd site_agentroom | cut -d: -f7)" in /usr/sbin/nologin|/bin/false) ;; *) fail 'runtime account may not have an interactive shell' ;; esac
  # Pre-add the group so the unmodified helper does not restart shared Caddy.
  sudo usermod -aG site_agentroom caddy
  sudo install -d -o michael -g site_agentroom -m 0750 "$SITE"
  mkdir -p "$SITE/releases/$REVISION" "$SITE/storage/artifacts" "$SITE/storage/logs" "$SITE/storage/workspaces"
  install_release
  [[ ! -e "$SITE/current" || -L "$SITE/current" ]] || fail 'current must be an owned release selector'
  ln -sfn "releases/$REVISION" "$SITE/current"
  for name in agentroom.conf dex-config.json github-run.sh supervisor.conf; do cp "$BUNDLE/$name" "$SITE/$name"; done
  cp -a "$BUNDLE/adapters" "$SITE/"
  mkdir -p "$SITE/secrets"
  cp -a "$BUNDLE/secrets/." "$SITE/secrets/"
  cp -a "$BUNDLE/artifacts/." "$SITE/storage/artifacts/"
  # Dex is non-root and reads only the site group; its numeric identity is host-local.
  jq --arg uid "$(id -u site_agentroom):$(id -g site_agentroom)" '.services.dex.user=$uid' "$BUNDLE/compose.json" > "$SITE/compose.json"
  sudo install -d -o root -g root -m 0755 /var/lib/agentroom-shared
  sudo install -d -o site_agentroom -g site_agentroom -m 0700 "$RUNTIME"
  # Existing private runtime files are replaced only while fenced and inactive.
  while IFS= read -r -d '' path; do
    relative="${path#"$BUNDLE/github/"}"
    if [[ -d "$path" ]]; then
      sudo install -d -o site_agentroom -g site_agentroom -m 0700 "$RUNTIME/$relative"
    else sudo install -o site_agentroom -g site_agentroom -m 0600 "$path" "$RUNTIME/$relative"; fi
  done < <(find "$BUNDLE/github" -mindepth 1 -print0 | sort -z)
  # This canonical helper also gracefully reloads shared php8.5-fpm by design.
  note 'Running canonical site hardener (includes its documented graceful PHP-FPM reload).'
  sudo bash "$HOME/harden-site.sh" agentroom site_agentroom
  write_marker provisioned
  rm -f "$SITE/.shared-host-restored"
  note 'Provisioned immutable code/private configuration; app and collector remain stopped.'
}

restore() {
  [[ ! -f "$SITE/.shared-host-activated" ]] || fail 'activation has fenced database restore permanently for this move'
  marker provisioned || fail 'provision this exact final bundle before restoring'
  require_stopped
  if marker restored; then note 'Exact snapshot already restored; no database changed.'; return; fi
  compose up --detach --wait postgres >/dev/null
  [[ "$(compose ps --status running --services)" == postgres ]] || fail 'only PostgreSQL may run during restore'
  for kind in app identity; do
    compose exec -T postgres pg_restore --list < "$BUNDLE/dumps/$kind.dump" >/dev/null 2>&1 || fail 'invalid database dump; no databases were reset'
  done
  # SQL is never an argument, environment value or host-side psql invocation.
  compose exec -T postgres psql -X -v ON_ERROR_STOP=1 -U "$PG_ADMIN" -d postgres < "$BUNDLE/restore.sql" >/dev/null
  for kind in app identity; do
    if [[ "$kind" == app ]]; then database="$APP_DB"; role="$APP_ROLE"; else database="$IDENTITY_DB"; role="$IDENTITY_ROLE"; fi
    printf 'DROP DATABASE IF EXISTS "%s" WITH (FORCE);\nCREATE DATABASE "%s" OWNER "%s";\n' "$database" "$database" "$role" |
      compose exec -T postgres psql -X -v ON_ERROR_STOP=1 -U "$PG_ADMIN" -d postgres >/dev/null
    compose exec -T postgres pg_restore --exit-on-error --no-owner --no-acl --role "$role" -U "$PG_ADMIN" --dbname "$database" < "$BUNDLE/dumps/$kind.dump" >/dev/null
  done
  [[ "$(compose exec -T postgres psql -X -At -U "$PG_ADMIN" -d "$APP_DB" -c "SELECT to_regclass('public.events') IS NOT NULL AND to_regclass('public.resources') IS NOT NULL")" == t ]] || fail 'application schema missing after restore'
  # Static-password Dex installations need not persist rows in password. The
  # original subject/hash is retained in the validated, unchanged static config.
  [[ "$(compose exec -T postgres psql -X -At -U "$PG_ADMIN" -d "$IDENTITY_DB" -c "SELECT to_regclass('public.password') IS NOT NULL AND to_regclass('public.keys') IS NOT NULL")" == t ]] || fail 'preserved identity schema missing after restore'
  write_marker restored
  note 'Both isolated databases restored; code, artifacts and durable queue are staged. No app ingestion started.'
}

ensure_route() {
  local port="$1" target="$2" state
  state="$(serve_state "$port" "$target")"
  [[ "$state" == owned ]] && return
  # Intent recorded before apply: interrupted retry may accept only the exact route.
  printf '%s\n' "$PUBLIC_URL" > "$SITE/.shared-host-serve-$port"
  chmod 0640 "$SITE/.shared-host-serve-$port"
  sudo tailscale serve --bg --https="$port" --yes "$target" >/dev/null
  [[ "$(serve_state "$port" "$target")" == owned ]] || fail 'own private Serve route failed validation'
}

wait_status() {
  local url="$1" wanted="$2" code count
  for ((count=0; count<45; count++)); do
    code="$(curl --silent --output /dev/null --write-out '%{http_code}' --max-time 2 "$url" || true)"
    [[ "$code" == "$wanted" ]] && return
    sleep 1
  done
  fail 'own service readiness/status check failed'
}

start() {
  if ! marker provisioned || ! marker restored; then fail 'exact bundle must be provisioned and restored before activation'; fi
  if [[ -f "$SITE/.shared-host-activated" ]]; then marker activated || fail 'another bundle already activated'; fi
  # This point is irreversible for restore even if a later service start fails.
  if [[ ! -f "$SITE/.shared-host-activated-at" ]]; then
    python3 -c 'import datetime; print(datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="microseconds").replace("+00:00", "Z"))' > "$SITE/.shared-host-activated-at"
    chmod 0640 "$SITE/.shared-host-activated-at"
  fi
  write_marker activated
  compose up --detach --wait postgres dex >/dev/null
  wait_status "http://127.0.0.1:$OIDC_PORT/dex/.well-known/openid-configuration" 200
  ensure_route 4445 http://127.0.0.1:4445
  ensure_route 4446 http://127.0.0.1:5557
  # Install only our own conf, enabling boot survival only after restore is fenced.
  sed 's/^autostart=false$/autostart=true/' "$SITE/supervisor.conf" > "$SITE/.supervisor-active.conf"
  if [[ -e "$SUPERVISOR" ]]; then
    [[ -f "$SITE/.shared-host-supervisor" ]] || fail 'existing Supervisor config lacks own apply marker'
  fi
  printf '%s\n' "$BUNDLE_DIGEST" > "$SITE/.shared-host-supervisor"
  sudo install -o root -g root -m 0644 "$SITE/.supervisor-active.conf" "$SUPERVISOR"
  rm "$SITE/.supervisor-active.conf"
  sudo supervisorctl reread >/dev/null
  sudo supervisorctl update "${PROGRAMS[@]}" >/dev/null
  for program in "${PROGRAMS[@]}"; do
    if [[ "$(supervisor_state "$program")" != RUNNING ]]; then sudo supervisorctl start "$program" >/dev/null; fi
    if [[ "$program" == agentroom-web ]]; then wait_status "http://127.0.0.1:$ADMIN_PORT/readyz" 204; fi
  done
  check
}

collector_healthy() {
  python3 -c '
import datetime, json, sys
try:
    def instant(value): return datetime.datetime.fromisoformat(value.replace("Z", "+00:00"))
    activated = instant(sys.argv[1]); status = json.load(sys.stdin)
    def fresh(value): return bool(value) and instant(value) >= activated
    projects = status.get("projects", [])
    healthy = (status.get("initialized") is True and fresh(status.get("last_collection")) and
        status.get("pending") == 0 and status.get("quarantined") == 0 and
        not status.get("collection_error") and not status.get("transport_error") and
        not status.get("needs_provisioning") and not status.get("unreadable") and bool(projects) and
        all(not row.get("delivery_error") and fresh(row.get("last_delivery")) for row in projects))
    sys.exit(0 if healthy else 1)
except (ValueError, TypeError, KeyError, AttributeError): sys.exit(1)
' "$1"
}

check_collector() {
  local count status
  for ((count=0; count<90; count++)); do
    status="$(sudo -u site_agentroom /usr/bin/python3 -B "$SITE/adapters/github/agentroom_github.py" --config "$RUNTIME/connector.toml" status)" || fail 'collector private state validation failed'
    if collector_healthy "$(cat "$SITE/.shared-host-activated-at")" <<< "$status"; then return; fi
    if ((count % 30 == 0)); then note 'Waiting for a new healthy GitHub collection and private API delivery from this host.'; fi
    sleep 2
  done
  fail 'GitHub has not completed healthy post-activation collection/delivery; source remains fenced for operator review'
}

check() {
  marker activated || fail 'this bundle has not been activated'
  local program pid uid port path
  for program in "${PROGRAMS[@]}"; do
    [[ "$(supervisor_state "$program")" == RUNNING ]] || fail 'own Supervisor service is not running'
    pid="$(sudo supervisorctl pid "$program")"
    [[ "$pid" =~ ^[1-9][0-9]*$ ]] || fail 'invalid Supervisor service PID'
    uid="$(ps -o uid= -p "$pid" | tr -d ' ')"
    [[ "$uid" == "$(id -u site_agentroom)" ]] || fail 'service runtime identity mismatch'
  done
  [[ "$(serve_state 4445 http://127.0.0.1:4445)" == owned && "$(serve_state 4446 http://127.0.0.1:5557)" == owned ]] || fail 'private Serve ownership mismatch'
  [[ "$(compose ps --status running --services | sort)" == $'dex\npostgres' ]] || fail 'own PostgreSQL/Dex containers are not healthy'
  for port in "$ADMIN_PORT" "$ADAPTER_PORT"; do
    ss -H -ltn "sport = :$port" | awk '{print $4}' | python3 -c 'import sys; rows=sys.stdin.read().splitlines(); assert rows and all(row.startswith("127.0.0.1:") for row in rows), "private listener escaped loopback"'
  done
  wait_status "http://127.0.0.1:$ADMIN_PORT/readyz" 204
  wait_status "$PUBLIC_URL/api/v1/projects" 401
  for path in /livez /readyz /metrics /mcp /api/v1/ingest /api/v1/mcp /admin/migrations; do wait_status "$PUBLIC_URL$path" 404; done
  [[ "$(curl --fail --silent --max-time 10 "$ISSUER/.well-known/openid-configuration" | jq -er '.issuer')" == "$ISSUER" ]] || fail 'preserved identity issuer check failed'
  check_collector
  note 'Own app/collector identities, both databases, private Serve, readiness and public API boundaries passed.'
}

check_preflight_markers() {
  local staged="${1:-}"
  if [[ -n "$staged" ]]; then
    # A final upload replaces the incoming directory before provision updates
    # the site's markers. Only these two checkpointed manifests may appear.
    [[ ! -e "$SITE/.shared-host-activated" ]] || fail 'activation already recorded; refusing provisioning recovery'
    require_hardener
    require_stopped
    if marker provisioned "$staged" && marker restored "$staged"; then
      provision_intent "$staged" || provision_intent || fail 'provisioning intent differs from the recovery checkpoint'
      return
    fi
    if ! marker provisioned || ! provision_intent; then fail 'final provisioning markers differ from the recovery checkpoint'; fi
    # Provision writes its marker before removing the old restore marker.
    if [[ -e "$SITE/.shared-host-restored" ]] && ! marker restored && ! marker restored "$staged"; then
      fail 'restore marker differs from the recovery checkpoint'
    fi
  else
    if ! marker provisioned || ! marker restored; then fail 'preflight requires this exact staged bundle to be provisioned and restored'; fi
    if [[ -f "$SITE/.shared-host-activated" ]]; then marker activated || fail 'preflight found a different active bundle'; fi
    if [[ "$(jq -r '.kind' "$BUNDLE/snapshot.json")" == rehearsal ]]; then require_hardener; fi
  fi
}

backup_site() {
  local out_root="$1"
  [[ -f "$SITE/.shared-host-activated" && -f "$SITE/compose.json" && -f "$SITE/dex-config.json" ]] || fail 'active shared-host installation is required for backup'
  python3 - "$out_root" <<'PY'
import pathlib, sys
raw = sys.argv[1]
root = pathlib.Path(raw)
if not root.is_absolute() or raw != str(root) or '..' in root.parts or root == pathlib.Path('/'):
    raise SystemExit('backup output directory must be a normalized absolute path')
for parent in [root, *root.parents]:
    if parent.is_symlink():
        raise SystemExit('backup output path may not contain symlinks')
PY
  local stamp dest pg_admin app_db identity_db current_release
  stamp="$(date -u +%Y%m%dT%H%M%SZ)"
  install -d -m 0700 "$out_root"
  dest="$out_root/agentroom-backup-$stamp"
  install -d -m 0700 "$dest"
  pg_admin="$(jq -er '.services.postgres.environment.POSTGRES_USER' "$SITE/compose.json")"
  app_db="$(jq -er '.services.postgres.environment.POSTGRES_DB' "$SITE/compose.json")"
  identity_db="$(jq -er '.storage.config.database' "$SITE/dex-config.json")"
  current_release="$(readlink "$SITE/current" 2>/dev/null || printf 'unknown')"

  compose exec -T postgres pg_dump -Fc -U "$pg_admin" -d "$app_db" > "$dest/app.dump"
  compose exec -T postgres pg_restore --list < "$dest/app.dump" >/dev/null || fail 'app database backup failed verification'
  compose exec -T postgres pg_dump -Fc -U "$pg_admin" -d "$identity_db" > "$dest/identity.dump"
  compose exec -T postgres pg_restore --list < "$dest/identity.dump" >/dev/null || fail 'identity database backup failed verification'
  chmod 0600 "$dest/app.dump" "$dest/identity.dump"

  if [[ -d "$SITE/storage/artifacts" ]]; then
    tar -czf "$dest/artifacts.tar.gz" -C "$SITE/storage" artifacts
    chmod 0600 "$dest/artifacts.tar.gz"
  fi
  if sudo test -f "$RUNTIME/state/queue.sqlite3"; then
    sudo -u site_agentroom python3 - "$RUNTIME/state/queue.sqlite3" "$dest/github-queue.sqlite3" <<'PY'
import sqlite3, sys
src = sqlite3.connect(f"file:{sys.argv[1]}?mode=ro", uri=True)
dst = sqlite3.connect(sys.argv[2])
try:
    src.backup(dst)
finally:
    dst.close()
    src.close()
PY
    sudo chown "$(id -un):$(id -gn)" "$dest/github-queue.sqlite3"
    chmod 0600 "$dest/github-queue.sqlite3"
  fi

  python3 - "$dest" "$stamp" "$current_release" "$app_db" "$identity_db" <<'PY'
import hashlib, json, pathlib, sys
dest = pathlib.Path(sys.argv[1])
files = []
for path in sorted(dest.iterdir()):
    if path.is_file() and path.name != 'manifest.json':
        files.append({'path': path.name, 'size': path.stat().st_size, 'sha256': hashlib.sha256(path.read_bytes()).hexdigest()})
manifest = {'schema': 1, 'created_at': sys.argv[2], 'release': sys.argv[3], 'databases': {'app': sys.argv[4], 'identity': sys.argv[5]}, 'files': files}
target = dest / 'manifest.json'
target.write_text(json.dumps(manifest, indent=2, sort_keys=True) + '\n')
target.chmod(0o600)
PY
  note "Verified shared-host backup written to $dest"
}

upgrade_release() {
  local artifact="$1" pubkey="$2" verifier="${3:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)/verify-artifact.sh}"
  [[ -f "$SITE/.shared-host-activated" && -L "$SITE/current" ]] || fail 'active shared-host installation is required for upgrade'
  [[ -f "$artifact" && ! -L "$artifact" && -f "$pubkey" && ! -L "$pubkey" && -f "$verifier" ]] || fail 'signed artifact, public key, and verify-artifact.sh are required'
  local admin_addr admin_port scratch revision release_digest previous
  admin_addr="$(awk -F= '$1=="AGENTROOM_ADMIN_ADDR"{gsub(/"/,"",$2); print $2}' "$SITE/agentroom.conf")"
  admin_port="${admin_addr##*:}"
  [[ "$admin_port" =~ ^[0-9]{4,5}$ ]] || fail 'unable to determine loopback admin port from agentroom.conf'

  scratch="$(mktemp -d "${TMPDIR:-/tmp}/agentroom-upgrade-XXXXXXXX")"
  mkdir "${scratch}/release"
  if ! TMPDIR="${scratch}" bash "$verifier" --artifact "$artifact" --public-key "$pubkey" --extract-to "${scratch}/release" >/dev/null; then
    rm -rf -- "$scratch"; fail 'signed release verification failed'
  fi
  revision="$(jq -er '.source_revision' "${scratch}/release/manifest.json")" || { rm -rf -- "$scratch"; fail 'release manifest missing source_revision'; }
  [[ "$revision" =~ ^[a-f0-9]{40}$ ]] || { rm -rf -- "$scratch"; fail 'invalid release source_revision'; }
  for rel in bin/agentroomd bin/agentroomctl web/index.html manifest.json; do
    [[ -s "${scratch}/release/$rel" ]] || { rm -rf -- "$scratch"; fail "verified release missing $rel"; }
  done
  release_digest="$(sha256sum "${scratch}/release/manifest.json" | awk '{print $1}')"

  if [[ -f "$SITE/.shared-host-release-$revision" ]]; then
    [[ "$(cat "$SITE/.shared-host-release-$revision")" == "$release_digest" ]] || { rm -rf -- "$scratch"; fail 'existing release provenance mismatch'; }
    diff -qr "${scratch}/release" "$SITE/releases/$revision" >/dev/null || { rm -rf -- "$scratch"; fail 'existing installed release differs'; }
  else
    mkdir -p "$SITE/releases/$revision"
    cp -a "${scratch}/release/." "$SITE/releases/$revision/"
    diff -qr "${scratch}/release" "$SITE/releases/$revision" >/dev/null || { rm -rf -- "$scratch"; fail 'release copy incomplete'; }
    sudo chown -R michael:site_agentroom "$SITE/releases/$revision"
    sudo chmod -R o-rwx "$SITE/releases/$revision"
    printf '%s\n' "$release_digest" > "$SITE/.shared-host-release-$revision"
    chmod 0640 "$SITE/.shared-host-release-$revision"
  fi
  rm -rf -- "$scratch"

  sudo -u site_agentroom "$SITE/releases/$revision/bin/agentroomctl" --config "$SITE/agentroom.conf" migrate up
  previous="$(readlink "$SITE/current")"
  ln -sfn "releases/$revision" "$SITE/current"
  sudo supervisorctl restart agentroom-web >/dev/null
  if ! wait_status "http://127.0.0.1:$admin_port/readyz" 204; then
    ln -sfn "$previous" "$SITE/current"
    sudo supervisorctl restart agentroom-web >/dev/null || true
    fail "upgraded release $revision failed readiness check; rolled back to $previous"
  fi
  note "Upgraded shared-host release to $revision (previous: $previous)."
}

main() {
  if [[ "${1:-}" == backup ]]; then
    [[ $# == 2 ]] || fail 'usage: shared-host.sh backup ABSOLUTE_OUTPUT_DIR'
    [[ "$(uname -s)" == Linux && "$(uname -m)" == x86_64 ]] || fail 'destination must be Linux/amd64'
    [[ "$(id -un)" == michael && "$(id -u)" != 0 ]] || fail 'run as michael; root execution is forbidden'
    validate_operator_files
    exec 9> "$HOME/.agentroom-shared-host.lock"
    flock -n 9 || fail 'another Agent Room operation is running'
    backup_site "$2"
    return
  fi
  if [[ "${1:-}" == upgrade ]]; then
    [[ $# == 3 || $# == 4 ]] || fail 'usage: shared-host.sh upgrade ABSOLUTE_ARTIFACT ABSOLUTE_PUBLIC_KEY [VERIFY_SCRIPT]'
    [[ "$(uname -s)" == Linux && "$(uname -m)" == x86_64 ]] || fail 'destination must be Linux/amd64'
    [[ "$(id -un)" == michael && "$(id -u)" != 0 ]] || fail 'run as michael; root execution is forbidden'
    validate_operator_files
    exec 9> "$HOME/.agentroom-shared-host.lock"
    flock -n 9 || fail 'another Agent Room operation is running'
    sudo -v
    safe_host_paths
    upgrade_release "$2" "$3" "${4:-}"
    return
  fi
  [[ $# == 2 || ( "$1" == preflight && ( $# == 3 || $# == 4 ) ) ]] || fail 'usage: shared-host.sh validate|provision|restore|start|check ABSOLUTE_BUNDLE; preflight ABSOLUTE_BUNDLE EXPECTED_MANIFEST_SHA256 [REHEARSAL_SHA256]; backup ABSOLUTE_OUTPUT_DIR; upgrade ABSOLUTE_ARTIFACT ABSOLUTE_PUBLIC_KEY'
  MODE="$1"; BUNDLE="$2"
  case "$MODE" in validate|preflight|provision|restore|start|check) ;; *) fail 'unknown mode' ;; esac
  command -v python3 >/dev/null || fail 'Python 3.11+ is required for bundle validation'
  BUNDLE_DIGEST="$(validate_bundle "$BUNDLE")" || fail 'bundle rejected before host mutation'
  if [[ "$MODE" == preflight ]]; then
    [[ $# -ge 3 && "$3" =~ ^[a-f0-9]{64}$ && "$BUNDLE_DIGEST" == "$3" ]] || fail 'preflight staged manifest differs from the uploaded checkpoint'
    if [[ $# == 4 ]]; then
      [[ "$4" =~ ^[a-f0-9]{64}$ && "$4" != "$3" ]] || fail 'invalid rehearsal checkpoint for recovery'
      jq -e '.kind == "final" and .source_fenced == true' "$BUNDLE/snapshot.json" >/dev/null || fail 'recovery requires a fenced final upload'
    fi
  fi
  if [[ "$MODE" == validate ]]; then note 'Bundle inventory, hashes, paths and shared-host configuration boundaries passed (signature is checked on Linux before provisioning).'; return; fi
  if [[ "$MODE" == start ]]; then
    jq -e '.schema == 1 and .kind == "final" and .source_fenced == true' "$BUNDLE/snapshot.json" >/dev/null || fail 'activation requires final snapshot from a fenced source'
  fi
  [[ "$(uname -s)" == Linux && "$(uname -m)" == x86_64 ]] || fail 'destination must be Linux/amd64'
  [[ "$(id -un)" == michael && "$(id -u)" != 0 ]] || fail 'run as michael; root execution is forbidden'
  REVISION="$(jq -r '.source_revision' "$BUNDLE/metadata.json")"
  PUBLIC_URL="$(jq -r '.public_url' "$BUNDLE/metadata.json")"; ISSUER="$(jq -r '.issuer' "$BUNDLE/metadata.json")"
  HTTP_PORT="$(jq -r '.ports.http' "$BUNDLE/metadata.json")"; OIDC_PORT="$(jq -r '.ports.oidc' "$BUNDLE/metadata.json")"
  PG_PORT="$(jq -r '.ports.postgres' "$BUNDLE/metadata.json")"; ADMIN_PORT="$(jq -r '.ports.admin' "$BUNDLE/metadata.json")"; ADAPTER_PORT="$(jq -r '.ports.adapter' "$BUNDLE/metadata.json")"
  APP_DB="$(jq -r '.app_database' "$BUNDLE/metadata.json")"; IDENTITY_DB="$(jq -r '.identity_database' "$BUNDLE/metadata.json")"
  APP_ROLE="$(jq -r '.app_role' "$BUNDLE/metadata.json")"; IDENTITY_ROLE="$(jq -r '.identity_role' "$BUNDLE/metadata.json")"; PG_ADMIN="$(jq -r '.postgres_admin' "$BUNDLE/metadata.json")"
  verify_release
  validate_operator_files
  # Serialize Michael's installer invocations without a global/shared-service lock.
  exec 9> "$HOME/.agentroom-shared-host.lock"
  flock -n 9 || fail 'another Agent Room migration command is running'
  preflight
  if [[ "$MODE" == preflight ]]; then
    check_preflight_markers "${4:-}"
    note 'Uploaded bundle, signature, host identity, site markers and sudo access verified. Later SSH sessions may prompt for sudo again.'
    return
  fi
  "$MODE"
}

if [[ "${BASH_SOURCE[0]:-}" == "$0" ]]; then main "$@"; fi
