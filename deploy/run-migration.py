#!/usr/bin/env python3
"""Michael-run Mac orchestration for the private shared-host migration.

No command runs on import. Stage rehearses without activating the app. Cutover
fences local writers before a fresh final export. Once remote activation might
have occurred, this tool never reimports data or automatically resumes hosting.
"""
from __future__ import annotations

import argparse
import contextlib
import datetime as dt
import fcntl
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import plistlib
import re
import shlex
import shutil
import stat
import subprocess
import sys
import tempfile
import time
import urllib.parse
import uuid

UID = 501
DOMAIN = f"gui/{UID}"
LABELS = ("local.agentroom-github", "local.agentroom-sentry", "local.agentroom")
TUNNEL = "local.agentroom-tunnel"
SSH_OPTIONS = ("-oBatchMode=yes", "-oStrictHostKeyChecking=yes")
PORTS = {"http": 4445, "oidc": 5557, "postgres": 55440, "admin": 54490, "adapter": 54491}


class MigrationError(Exception):
    """Only fixed messages, never process output or credential values."""


def require(ok, message):
    if not ok:
        raise MigrationError(message)


def private_directory(path):
    for parent in (path, *path.parents):
        require(not parent.is_symlink(), "private migration paths may not contain symbolic links")
    path.mkdir(mode=0o700, parents=True, exist_ok=True)
    info = path.stat()
    require(info.st_uid == os.getuid() and stat.S_IMODE(info.st_mode) & 0o077 == 0,
            "migration directories must be owned by this operator with mode 0700")


def read_json(path, private=True):
    info = path.lstat()
    require(stat.S_ISREG(info.st_mode) and info.st_nlink == 1 and info.st_uid == os.getuid(),
            "migration control files must be owned regular files without links")
    require(not private or stat.S_IMODE(info.st_mode) & 0o077 == 0,
            "migration control files require mode 0600")
    require(info.st_size <= 32 * 1024 * 1024, "migration control file exceeds its bound")
    def pairs(items):
        value = {}
        for key, item in items:
            require(key not in value, "duplicate migration JSON key")
            value[key] = item
        return value
    return json.loads(path.read_text(), object_pairs_hook=pairs)


def digest(path):
    value = hashlib.sha256()
    with path.open('rb') as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b''):
            value.update(block)
    return value.hexdigest()


def write_private(path, data):
    private_directory(path.parent)
    require(not path.is_symlink(), "refusing a linked migration checkpoint")
    fd, temporary = tempfile.mkstemp(prefix='.migration-', dir=path.parent)
    try:
        with os.fdopen(fd, 'wb') as stream:
            stream.write(data)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
        descriptor = os.open(path.parent, os.O_RDONLY)
        try:
            os.fsync(descriptor)
        finally:
            os.close(descriptor)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


@contextlib.contextmanager
def process_lock(directory):
    private_directory(directory)
    path = directory / 'orchestrator.lock'
    fd = os.open(path, os.O_RDWR | os.O_CREAT | os.O_NOFOLLOW, 0o600)
    try:
        info = os.fstat(fd)
        require(stat.S_ISREG(info.st_mode) and info.st_uid == os.getuid() and not info.st_mode & 0o077,
                "migration lock must be a private owned regular file")
        try:
            fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise MigrationError("another migration invocation owns the checkpoint") from None
        yield
    finally:
        os.close(fd)


class Bundle:
    def __init__(self, path):
        self.path = Path(path).absolute()
        require(self.path == self.path.resolve(), "bundle path must be normalized and contain no links")
        require(self.path.is_dir(), "prepare the private bundle before running migration")
        private_directory(self.path)
        self.meta = read_json(self.path / 'metadata.json')
        self.operator = read_json(self.path / 'operator.json')
        self.snapshot = read_json(self.path / 'snapshot.json')
        require(self.meta.get('schema') == 1 and re.fullmatch('[a-f0-9]{40}', self.meta.get('source_revision', '')),
                "bundle requires an exact release revision")
        require(self.meta.get('ports') == PORTS, "migration ports differ from the approved private topology")
        public = urllib.parse.urlsplit(self.meta.get('public_url', ''))
        require(public.scheme == 'https' and public.hostname and public.hostname.endswith('.ts.net') and public.port == 4445 and not public.username and not public.password and not public.path and not public.query and not public.fragment,
                "destination must be a private Tailscale HTTPS origin on port 4445")
        require(self.meta.get('issuer') == f'https://{public.hostname}:4446/dex', "identity issuer differs from the private destination")
        require(self.snapshot.get('schema') == 1 and self.snapshot.get('kind') in ('rehearsal', 'final') and self.snapshot.get('source_fenced') is (self.snapshot['kind'] == 'final'),
                "bundle snapshot proof is invalid")
        self.host = self.operator.get('ssh_host', '')
        require(re.fullmatch('[A-Za-z0-9][A-Za-z0-9_.-]{0,253}', self.host), "use a simple trusted SSH host alias")
        self.repo = Path(self.operator.get('repo', ''))
        self.local = Path(self.operator.get('local', ''))
        for path in (self.repo, self.local):
            require(path.is_absolute() and path == path.resolve() and '..' not in path.parts,
                    "operator paths must be normalized absolute paths without links")
        require(self.repo == Path(__file__).resolve().parents[1], "bundle must use this repository's export helper")
        require(self.local.is_dir() and self.local != self.repo and self.path != self.local,
                "local runtime and bundle paths must be separate directories")
        self.verify()

    def verify(self):
        manifest = read_json(self.path / 'manifest.json')
        require(manifest.get('schema') == 1 and isinstance(manifest.get('files'), list), "invalid bundle manifest")
        expected = {}
        for item in manifest['files']:
            relative = item.get('path')
            require(isinstance(relative, str) and re.fullmatch('[A-Za-z0-9][A-Za-z0-9._/+@-]*', relative), "unsafe manifest path")
            parsed = PurePosixPath(relative)
            require(not parsed.is_absolute() and all(part not in ('.', '..') for part in relative.split('/')) and relative not in expected,
                    "duplicate or escaping manifest path")
            require(type(item.get('size')) is int and item['size'] >= 0 and re.fullmatch('[a-f0-9]{64}', item.get('sha256', '')),
                    "invalid manifest file digest or size")
            expected[relative] = (item['size'], item['sha256'])
        found = {}
        for path in self.path.rglob('*'):
            info = path.lstat()
            require(info.st_uid == os.getuid() and not info.st_mode & 0o022 and (stat.S_ISDIR(info.st_mode) or stat.S_ISREG(info.st_mode)),
                    "bundle contains foreign ownership, writable paths, links or special files")
            if path.is_file() and path != self.path / 'manifest.json':
                require(info.st_nlink == 1, "bundle hard links are forbidden")
                found[path.relative_to(self.path).as_posix()] = (info.st_size, digest(path))
        require(found == expected, "bundle inventory, size or digest does not match the manifest")
        required = {'metadata.json', 'operator.json', 'snapshot.json', 'shared-host.sh', 'dumps/app.dump', 'dumps/identity.dump', 'github/state/queue.sqlite3'}
        require(required <= expected.keys(), "bundle lacks required migration inputs")
        self.manifest_digest = digest(self.path / 'manifest.json')

    @property
    def identity(self):
        return {'source_revision': self.meta['source_revision'], 'ssh_host': self.host,
                'bundle': str(self.path), 'repo': str(self.repo), 'local': str(self.local),
                'public_url': self.meta.get('public_url'), 'issuer': self.meta.get('issuer'), 'ports': PORTS}


class Runner:
    def __init__(self, log):
        self.log = log

    def run(self, args, *, cwd=None, interactive=False):
        # Remote verbs retain the real terminal for sudo. Helpers print operational
        # messages only; secrets are files, never arguments or streamed input.
        result = subprocess.run(args, cwd=cwd, text=True, check=False,
                                stdout=None if interactive else subprocess.PIPE,
                                stderr=None if interactive else subprocess.PIPE)
        if not interactive:
            private_directory(self.log.parent)
            fd = os.open(self.log, os.O_WRONLY | os.O_APPEND | os.O_CREAT | os.O_NOFOLLOW, 0o600)
            info = os.fstat(fd)
            if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o077:
                os.close(fd)
                raise MigrationError("migration log must be a private owned regular file")
            with os.fdopen(fd, 'a') as output:
                output.write(result.stdout or '')
                output.write(result.stderr or '')
        return result


class Migration:
    def __init__(self, bundle, runner=None):
        self.bundle = bundle
        self.directory = bundle.path.parent
        self.checkpoint = self.directory / 'orchestrator.json'
        self.runner = runner or Runner(self.directory / 'logs/orchestrator.log')
        if self.checkpoint.exists():
            self.state = read_json(self.checkpoint)
            require(self.state.get('schema') == 1 and self.state.get('identity') == bundle.identity,
                    "checkpoint belongs to another host, revision or runtime; explicit migration review required")
        else:
            self.state = {'schema': 1, 'identity': bundle.identity}
        self.plists = {
            'local.agentroom': bundle.local / 'local.agentroom.plist',
            'local.agentroom-sentry': bundle.local / 'sentry/local.agentroom-sentry.plist',
            'local.agentroom-github': bundle.local / 'github/local.agentroom-github.plist',
        }
        self.launch_agents = Path.home() / 'Library/LaunchAgents'

    def save(self, phase=None, **fields):
        self.state.update(fields)
        if phase:
            self.state['phase'] = phase
        write_private(self.checkpoint, (json.dumps(self.state, sort_keys=True, indent=2) + '\n').encode())

    def command(self, args, *, cwd=None, interactive=False):
        result = self.runner.run(args, cwd=cwd, interactive=interactive)
        require(result.returncode == 0, "migration command failed; inspect the private checkpoint/log; no hosting rollback was attempted")
        return result.stdout or ''

    def ssh(self, words, *, tty=False):
        args = ['/usr/bin/ssh', *(['-t'] if tty else []), *SSH_OPTIONS, '--', self.bundle.host, shlex.join(words)]
        return self.command(args, interactive=tty)

    def remote_bundle(self):
        if not self.state.get('remote_bundle'):
            home = self.ssh(['sh', '-c', 'printf "%s\\n" "$HOME"']).strip()
            require(re.fullmatch('/[A-Za-z0-9_./-]+', home) and '..' not in PurePosixPath(home).parts and home != '/',
                    "destination home must be a normalized safe absolute directory")
            path = str(PurePosixPath(home) / 'agentroom-migration' / self.bundle.meta['source_revision'])
            self.save(remote_bundle=path)
        existing = self.state['remote_bundle']
        parsed = PurePosixPath(existing)
        require(re.fullmatch('/[A-Za-z0-9_./-]+', existing) and str(parsed) == existing and '..' not in parsed.parts and parsed.name == self.bundle.meta['source_revision'] and parsed.parent.name == 'agentroom-migration',
                "checkpoint contains an invalid private incoming path")
        return self.state['remote_bundle']

    def remote(self, mode):
        require(mode in ('preflight', 'provision', 'restore', 'start', 'check'), "unknown remote helper operation")
        expected = self.state.get('uploaded_manifest', '') if mode == 'preflight' else None
        require(mode != 'preflight' or isinstance(expected, str) and re.fullmatch('[a-f0-9]{64}', expected),
                "preflight requires the exact previously uploaded manifest checkpoint")
        require(mode != 'preflight' or isinstance(self.state.get('remote_bundle'), str),
                "preflight requires the previously staged incoming path")
        path = self.remote_bundle()
        if expected is not None:
            # The staged bundle must remain byte-for-byte bound to its original
            # manifest. Run this checkout's reviewed preflight code inline rather
            # than replacing the old staged helper or uploading a new bundle.
            helper = self.bundle.repo / 'deploy/shared-host.sh'
            require(helper.is_file() and not helper.is_symlink(), "the reviewed local shared-host helper is unavailable")
            require(helper.stat().st_size <= 64 * 1024, "shared-host preflight helper exceeds its inline bound")
            source = helper.read_text()
            require(len(source.encode()) <= 64 * 1024, "shared-host preflight helper exceeds its inline bound")
            program = source + '\nmain "$@"\n'
            arguments = ['bash', '-c', program, 'agentroom-preflight', mode, path, expected]
        else:
            arguments = ['bash', path + '/shared-host.sh', mode, path]
        self.ssh(arguments, tty=True)

    def backup(self, name):
        snapshots = self.directory / 'snapshots'
        private_directory(snapshots)
        target = snapshots / (name + '-' + self.bundle.manifest_digest)
        if not target.exists():
            temporary = snapshots / ('.copy-' + uuid.uuid4().hex)
            shutil.copytree(self.bundle.path, temporary, symlinks=False)
            temporary.chmod(0o700)
            temporary.rename(target)
        require(Bundle(target).manifest_digest == self.bundle.manifest_digest,
                "snapshot conflicts with the current bundle")
        return str(target)

    def upload(self):
        path = self.remote_bundle()
        if self.state.get('uploaded_manifest') == self.bundle.manifest_digest:
            return
        parent = str(PurePosixPath(path).parent)
        temporary = path + '.incoming-' + uuid.uuid4().hex
        prepare = '''import os,pathlib,stat,sys
parent,temp=map(pathlib.Path,sys.argv[1:])
for p in [parent,*parent.parents]:
 if p.is_symlink(): raise SystemExit('Refusing linked incoming directory')
parent.mkdir(parents=True,exist_ok=True,mode=0o700)
s=parent.stat()
if s.st_uid!=os.getuid() or stat.S_IMODE(s.st_mode)&0o077: raise SystemExit('Incoming parent must be private and owned')
temp.mkdir(mode=0o700)
'''
        self.ssh(['python3', '-c', prepare, parent, temporary])
        self.command(['/usr/bin/scp', '-rp', *SSH_OPTIONS, '--', str(self.bundle.path) + '/.', self.bundle.host + ':' + temporary + '/'])
        swap = '''import os,pathlib,stat,sys
src,dest=map(pathlib.Path,sys.argv[1:3])
for p in (src,dest):
 if p.is_symlink(): raise SystemExit('Refusing linked incoming bundle')
 if p.exists() and (p.stat().st_uid!=os.getuid() or stat.S_IMODE(p.stat().st_mode)&0o077): raise SystemExit('Incoming bundle must be private and owned')
if dest.exists(): dest.rename(str(dest)+'.previous-'+sys.argv[3])
src.rename(dest)
'''
        self.ssh(['python3', '-c', swap, temporary, path, uuid.uuid4().hex])
        self.save(uploaded_manifest=self.bundle.manifest_digest)

    def loaded(self, label):
        result = self.runner.run(['/bin/launchctl', 'print', DOMAIN + '/' + label])
        require(result.returncode in (0, 113), "cannot verify the local launch agent state")
        return result.returncode == 0

    def stop(self, label):
        require(label in LABELS, "refusing to stop an unrelated launch agent")
        self.command(['/bin/launchctl', 'disable', DOMAIN + '/' + label])
        before = self.runner.run(['/bin/launchctl', 'print', DOMAIN + '/' + label])
        require(before.returncode in (0, 113), "cannot inspect the owned writer before fencing")
        pid = re.search(r'^\s*pid = ([1-9][0-9]*)\s*$', before.stdout or '', re.M)
        if before.returncode == 0:
            self.command(['/bin/launchctl', 'bootout', DOMAIN + '/' + label])
        # bootout can return while launchd is still tearing down the service.
        # Confirm both label removal and process exit within one bounded wait.
        deadline = time.monotonic() + 30
        while self.loaded(label):
            require(time.monotonic() < deadline,
                    f"local writer {label} remains loaded after shutdown timeout; final export is forbidden")
            time.sleep(0.25)
        if pid:
            while time.monotonic() < deadline:
                alive = self.runner.run(['/bin/kill', '-0', pid[1]])
                if alive.returncode == 1:
                    return
                require(alive.returncode == 0, "cannot verify the old writer exited")
                time.sleep(0.5)
            raise MigrationError("old writer process has not exited; final export is forbidden")

    def start_sentry(self):
        path = self.plists['local.agentroom-sentry']
        payload = plistlib.loads(path.read_bytes())
        require(payload.get('Label') == 'local.agentroom-sentry', "Sentry launch agent label mismatch")
        self.command(['/bin/launchctl', 'enable', DOMAIN + '/local.agentroom-sentry'])
        if not self.loaded('local.agentroom-sentry'):
            self.command(['/bin/launchctl', 'bootstrap', DOMAIN, str(path)])
        require(self.loaded('local.agentroom-sentry'), "local Sentry publisher failed to resume")
        self.save(sentry_resumed=True)

    def tunnel(self):
        private_directory(self.directory / 'logs')
        path = self.directory / 'local.agentroom-tunnel.plist'
        program = ['/usr/bin/ssh', '-NT', *SSH_OPTIONS, '-oExitOnForwardFailure=yes',
                   '-oServerAliveInterval=30', '-oServerAliveCountMax=3',
                   '-oControlMaster=no', '-oControlPath=none',
                   '-L127.0.0.1:54491:127.0.0.1:54491', self.bundle.host]
        payload = {'Label': TUNNEL, 'ProgramArguments': program, 'KeepAlive': True,
                   'RunAtLoad': True, 'ThrottleInterval': 10, 'Umask': 0o077,
                   'StandardOutPath': str(self.directory / 'logs/tunnel.stdout.log'),
                   'StandardErrorPath': str(self.directory / 'logs/tunnel.stderr.log')}
        encoded = plistlib.dumps(payload)
        if path.exists():
            require(not path.is_symlink() and path.read_bytes() == encoded, "existing tunnel configuration differs; inspect it explicitly")
        else:
            require(not self.loaded(TUNNEL), "another launch agent already owns the tunnel label")
            write_private(path, encoded)
        require(not any(p.is_symlink() for p in (self.launch_agents, *self.launch_agents.parents)),
                "LaunchAgents path may not contain symbolic links")
        self.launch_agents.mkdir(parents=True, exist_ok=True, mode=0o700)
        require(self.launch_agents.stat().st_uid == os.getuid() and not self.launch_agents.stat().st_mode & 0o022,
                "LaunchAgents directory must be owned and not writable by others")
        link = self.launch_agents / (TUNNEL + '.plist')
        if link.is_symlink():
            require(os.readlink(link) == str(path), "existing tunnel LaunchAgent belongs to another configuration")
        else:
            require(not link.exists(), "existing tunnel LaunchAgent must be reviewed explicitly")
            link.symlink_to(path)
        self.command(['/bin/launchctl', 'enable', DOMAIN + '/' + TUNNEL])
        if not self.loaded(TUNNEL):
            self.command(['/bin/launchctl', 'bootstrap', DOMAIN, str(path)])
        deadline = time.monotonic() + 30
        while time.monotonic() < deadline:
            result = self.runner.run(['/bin/launchctl', 'print', DOMAIN + '/' + TUNNEL])
            match = re.search(r'^\s*pid = ([1-9][0-9]*)\s*$', result.stdout or '', re.M) if result.returncode == 0 else None
            if match:
                listener = self.runner.run(['/usr/sbin/lsof', '-nP', '-t', '-a', '-p', match[1], '-iTCP@127.0.0.1:54491', '-sTCP:LISTEN'])
                if listener.returncode == 0 and listener.stdout.strip() == match[1]:
                    self.save(tunnel_ready=True)
                    return
            time.sleep(0.5)
        raise MigrationError("tunnel did not own the private listener; local Sentry remains stopped")

    def stage(self):
        require(not self.state.get('fence_started') and not self.state.get('activation_possible'),
                "cutover has begun; use cutover/check recovery, never a new rehearsal")
        if self.state.get('stage_complete'):
            return
        require(self.bundle.snapshot.get('kind') == 'rehearsal', "stage requires the initial rehearsal snapshot")
        self.save('staging', stage_backup=self.backup('stage'))
        self.upload()
        self.remote('provision')
        self.remote('restore')
        self.save('staged', stage_complete=True)

    def check(self):
        require(self.state.get('activation_possible'), "remote activation has not been attempted")
        require(self.bundle.manifest_digest == self.state.get('final_manifest'), "final bundle changed after activation; inspect the checkpoint")
        self.save('checking', remote_check_passed=False)
        self.remote('check')
        self.save('checked', remote_check_passed=True)

    def cutover(self):
        require(self.state.get('stage_complete'), "complete stage before cutover")
        require(not self.state.get('retired'), "local retirement is already complete")
        if self.state.get('cutover_complete'):
            self.check()
            self.deployment_handoff()
            return
        # Authenticate and verify the staged target before changing any local
        # writer. Sudo timestamps can be TTY-scoped: later sessions may prompt
        # again, and this check never promises cached authority across them.
        self.remote('preflight')
        self.save('fencing', fence_started=True)
        for label in LABELS:
            self.stop(label)
        self.save('fenced')
        if not self.state.get('activation_possible'):
            if not self.state.get('final_exported'):
                self.command([sys.executable, str(self.bundle.repo / 'deploy/prepare-migration.py'), 'export', '--bundle', str(self.bundle.path), '--final'], cwd=self.bundle.repo)
                refreshed = Bundle(self.bundle.path)
                require(refreshed.identity == self.bundle.identity and refreshed.snapshot.get('kind') == 'final' and refreshed.snapshot.get('source_fenced') is True,
                        "final export did not preserve identity and prove the local fence")
                self.bundle = refreshed
                self.save('exported', final_exported=True, final_manifest=refreshed.manifest_digest, final_backup=self.backup('final'))
            require(self.bundle.manifest_digest == self.state['final_manifest'], "final bundle changed; do not reimport without operator review")
            self.upload()
            self.remote('provision')
            self.remote('restore')
            self.save('restored', final_restore_complete=True)
            # Persist before SSH: an interrupted response cannot prove no writes occurred.
            self.save('activation-possible', activation_possible=True)
        require(self.state.get('final_restore_complete'), "final restore is not confirmed")
        require(self.bundle.manifest_digest == self.state.get('final_manifest'), "final bundle changed; remote activation cannot be resumed blindly")
        self.remote('start')
        self.check()
        self.tunnel()
        self.start_sentry()
        self.save('cutover-complete', cutover_complete=True)
        self.deployment_handoff()

    def retire(self, confirmation):
        require(confirmation == 'REMOTE_BROWSER_ACCEPTED', "retirement requires explicit REMOTE_BROWSER_ACCEPTED confirmation")
        require(self.state.get('cutover_complete') and self.state.get('remote_check_passed'), "remote cutover and checks must complete before local retirement")
        self.check()
        require(self.loaded(TUNNEL) and self.loaded('local.agentroom-sentry'), "keep the Sentry tunnel and publisher supervised before retirement")
        if self.state.get('retired'):
            self.deployment_handoff()
            return
        executable, routes = self.retire_routes()
        self.save('retiring', browser_accepted=True)
        for label in ('local.agentroom-github', 'local.agentroom'):
            self.stop(label)
        self.command(['docker', 'compose', '-f', str(self.bundle.local / 'compose.json'), 'stop', 'dex', 'postgres'])
        self.save(local_databases_stopped=True)
        for port in routes:
            self.command([executable, 'serve', '--yes', f'--https={port}', 'off'])
        self.save('retired', retired=True)
        self.deployment_handoff()

    def deployment_handoff(self):
        original = self.bundle.local / 'deployment.json'
        snapshot = self.directory / 'snapshots/deployment-before-migration.json'
        if not snapshot.exists():
            if original.exists():
                read_json(original)
                write_private(snapshot, original.read_bytes())
            else:
                write_private(snapshot, b'{"previous_record_absent":true}\n')
        else:
            read_json(snapshot)
        retired = bool(self.state.get('retired'))
        record = {'schema': 1, 'kind': 'shared-host-migration',
                  'authoritative_checkpoint': str(self.checkpoint), 'previous_local_snapshot': str(snapshot),
                  'phase': 'remote-browser-accepted-local-retired' if retired else 'remote-checked-browser-acceptance-pending',
                  'active': {'public_url': self.bundle.meta['public_url'], 'issuer': self.bundle.meta['issuer'],
                             'source_revision': self.bundle.meta['source_revision'], 'supervision': 'supervisor'},
                  'local': {'hosting': 'fenced', 'retired': retired, 'sentry': 'launchd-ssh-tunnel'}}
        write_private(original, (json.dumps(record, sort_keys=True, indent=2) + '\n').encode())
        self.save(deployment_handoff=True)

    def retire_routes(self):
        executable = shutil.which('tailscale')
        require(executable, "Tailscale CLI is required to remove only the old local routes")
        source = (self.bundle.local / 'agentroom.conf').read_text()
        values = [shlex.split(line.partition('=')[2]) for line in source.splitlines() if line.startswith('AGENTROOM_PUBLIC_URL=')]
        require(len(values) == 1 and len(values[0]) == 1, "local public URL must be explicit before retirement")
        parsed = urllib.parse.urlsplit(values[0][0])
        require(parsed.scheme == 'https' and parsed.hostname and parsed.port == 4445, "local Serve identity is ambiguous")
        compose = read_json(self.bundle.local / 'compose.json')
        require(set(compose.get('services', {})) == {'postgres', 'dex'}, "local compose scope must contain only own PostgreSQL/Dex")
        ports = compose['services']['dex'].get('ports', [])
        require(len(ports) == 1 and isinstance(ports[0], str), "local Dex mapping must be explicit")
        port = re.fullmatch(r'127\.0\.0\.1:([0-9]{4,5}):5556', ports[0])
        require(port, "local Dex listener must remain loopback")
        before = json.loads(self.command([executable, 'serve', 'status', '--json']))
        targets = {4445: 'http://127.0.0.1:4445', 4446: 'http://127.0.0.1:' + port[1]}
        def mentions(value, owned_port):
            if isinstance(value, dict):
                return any(key == owned_port or key.endswith(':' + owned_port) or mentions(item, owned_port) for key, item in value.items())
            return isinstance(value, list) and any(mentions(item, owned_port) for item in value)
        routes = []
        for number, target in targets.items():
            text = str(number)
            key = parsed.hostname + ':' + text
            matches = [name for name in before.get('Web', {}) if name.endswith(':' + text)]
            other = {name: value for name, value in before.items() if name not in ('TCP', 'Web', 'AllowFunnel')}
            require(not mentions(other, text) and not any(value and name.endswith(':' + text) for name, value in before.get('AllowFunnel', {}).items()),
                    "old local Serve port has ambiguous ownership or public Funnel")
            tcp = before.get('TCP', {}).get(text)
            if tcp is None and not matches:
                continue
            require(tcp == {'HTTPS': True} and matches == [key] and before['Web'][key] == {'Handlers': {'/': {'Proxy': target}}},
                    "old local Serve mapping differs; neighboring routes were not changed")
            routes.append(number)
        write_private(self.directory / 'snapshots/retire-serve.json', (json.dumps(before, sort_keys=True) + '\n').encode())
        return executable, routes


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--bundle', type=Path, required=True)
    parser.add_argument('--confirm', choices=('REMOTE_BROWSER_ACCEPTED',))
    parser.add_argument('command', choices=('stage', 'cutover', 'check', 'retire'))
    args = parser.parse_args()
    try:
        require(sys.platform == 'darwin' and os.getuid() == UID,
                "run this operator tool as the established Mac user, never as root or on the destination")
        require(sys.stdin.isatty(), "run this operator tool from an interactive terminal for destination sudo")
        os.umask(0o077)
        bundle = Bundle(args.bundle)
        with process_lock(bundle.path.parent):
            migration = Migration(bundle)
            if args.command == 'retire':
                migration.retire(args.confirm)
            else:
                require(args.confirm is None, "browser confirmation applies only to retire")
                getattr(migration, args.command)()
        print('Migration step complete. Local data and backups are retained; browser acceptance and retirement remain explicit.')
        return 0
    except MigrationError as error:
        # MigrationError contains fixed operational messages, never subprocess
        # output or private input. Keep unexpected exception details redacted.
        print(f'Migration step failed: {error}. No automatic hosting rollback or source resume was attempted.', file=sys.stderr)
        return 1
    except (OSError, ValueError, KeyError, TypeError, plistlib.InvalidFileException):
        print('Migration step failed. Inspect the private checkpoint/log. No automatic hosting rollback or source resume was attempted.', file=sys.stderr)
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
