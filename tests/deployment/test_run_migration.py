"""Operator workflow tests; commands and runtime paths are disposable fixtures."""
from __future__ import annotations

import importlib.util
import json
import os
from pathlib import Path
import plistlib
import shlex
import sys
import tempfile
import types
import unittest
from unittest.mock import patch

SOURCE = Path(__file__).resolve().parents[2] / 'deploy/run-migration.py'
SPEC = importlib.util.spec_from_file_location('run_migration', SOURCE)
operator = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = operator
SPEC.loader.exec_module(operator)


def write(path, value):
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    path.write_bytes(value if isinstance(value, bytes) else value.encode())
    path.chmod(0o600)


def seal(root):
    files = [{'path': path.relative_to(root).as_posix(), 'size': path.stat().st_size, 'sha256': operator.digest(path)}
             for path in sorted(root.rglob('*')) if path.is_file() and path != root / 'manifest.json']
    write(root / 'manifest.json', json.dumps({'schema': 1, 'files': files}))


class Commands:
    def __init__(self, fixture):
        self.fixture = fixture
        self.calls, self.events = [], []
        self.loaded = set(operator.LABELS)
        self.disabled = set()
        self.fail = None
        self.final_proof = True
        self.serve = {'TCP': {'443': {'HTTPS': True}, '4445': {'HTTPS': True}, '4446': {'HTTPS': True}},
                      'Web': {'local.fixture.ts.net:443': {'Handlers': {'/': {'Proxy': 'http://127.0.0.1:9999'}}},
                              'local.fixture.ts.net:4445': {'Handlers': {'/': {'Proxy': 'http://127.0.0.1:4445'}}},
                              'local.fixture.ts.net:4446': {'Handlers': {'/': {'Proxy': 'http://127.0.0.1:5556'}}}}}

    def run(self, args, **kwargs):
        self.calls.append((list(args), kwargs))
        name = Path(args[0]).name
        event, output, code = name, '', 0
        if name == 'ssh':
            command = shlex.split(args[-1])
            if command[0] == 'sh':
                event, output = 'remote-home', '/home/fixture\n'
            elif command[0] == 'bash':
                event = 'remote:' + (command[4] if command[1] == '-c' else command[2])
            else:
                event = 'remote-upload-helper'
        elif name == 'scp':
            event = 'upload'
        elif name == 'launchctl':
            action = args[1]
            if action == 'bootstrap':
                label = plistlib.loads(Path(args[3]).read_bytes())['Label']
            else:
                label = args[2].split('/', 2)[-1]
            event = action + ':' + label
            if action == 'print':
                code = 0 if label in self.loaded else 113
                output = 'pid = 8123\n' if code == 0 else ''
            elif action == 'disable':
                self.disabled.add(label)
            elif action == 'enable':
                self.disabled.discard(label)
            elif action == 'bootout' and self.fail != event:
                self.loaded.discard(label)
            elif action == 'bootstrap' and self.fail != event:
                if label in self.disabled:
                    code = 5
                else:
                    self.loaded.add(label)
        elif name == 'kill':
            event, code = 'process-exited', 1
        elif name == 'lsof':
            event, output = 'tunnel-listener', '8123\n'
        elif 'export' in args:
            event = 'export'
            assert all(label in self.disabled and label not in self.loaded for label in operator.LABELS)
            assert '--final' in args
            if self.final_proof:
                write(self.fixture.bundle / 'snapshot.json', json.dumps({'schema': 1, 'kind': 'final', 'source_fenced': True}))
                seal(self.fixture.bundle)
        elif name == 'docker':
            event = 'docker-stop'
        elif name == 'tailscale':
            if 'status' in args:
                event, output = 'serve-inspect', json.dumps(self.serve)
            else:
                event = 'serve-off:' + next(value.split('=')[1] for value in args if value.startswith('--https='))
        self.events.append(event)
        if self.fail == event:
            code = 9
        return types.SimpleNamespace(returncode=code, stdout=output, stderr='fixture-private-error' if code else '')


class MigrationTest(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix='agentroom-orchestrator-test-')
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name).resolve()
        self.local, self.bundle = self.root / 'local', self.root / 'migration/bundle'
        self.local.mkdir(mode=0o700)
        self.bundle.mkdir(parents=True, mode=0o700)
        self.bundle.parent.chmod(0o700)
        self.home = self.root / 'home'
        self.addCleanup(patch.stopall)
        patch.object(operator.Path, 'home', return_value=self.home).start()
        self.meta = {'schema': 1, 'source_revision': 'a' * 40, 'ports': operator.PORTS,
                     'public_url': 'https://remote.fixture.ts.net:4445', 'issuer': 'https://remote.fixture.ts.net:4446/dex'}
        self.control = {'ssh_host': 'fixture-host', 'repo': str(SOURCE.parents[1]), 'local': str(self.local)}
        write(self.bundle / 'metadata.json', json.dumps(self.meta))
        write(self.bundle / 'operator.json', json.dumps(self.control))
        write(self.bundle / 'snapshot.json', json.dumps({'schema': 1, 'kind': 'rehearsal', 'source_fenced': False}))
        for relative in ('shared-host.sh', 'dumps/app.dump', 'dumps/identity.dump', 'github/state/queue.sqlite3'):
            write(self.bundle / relative, 'fixture-private-migration-content')
        write(self.local / 'agentroom.conf', 'AGENTROOM_PUBLIC_URL="https://local.fixture.ts.net:4445"\n')
        write(self.local / 'compose.json', json.dumps({'services': {'postgres': {}, 'dex': {'ports': ['127.0.0.1:5556:5556']}}}))
        self.old_deployment = b'{"original_local_provenance":"fixture","source_revision":"old"}\n'
        write(self.local / 'deployment.json', self.old_deployment)
        self.library = self.home / 'Library/LaunchAgents'
        self.library.mkdir(parents=True, mode=0o700)
        for label, relative in (('local.agentroom', 'local.agentroom.plist'), ('local.agentroom-sentry', 'sentry/local.agentroom-sentry.plist'), ('local.agentroom-github', 'github/local.agentroom-github.plist')):
            path = self.local / relative
            write(path, plistlib.dumps({'Label': label, 'ProgramArguments': ['/bin/true']}))
            (self.library / (label + '.plist')).symlink_to(path)
        seal(self.bundle)
        self.commands = Commands(self)
        self.migration = operator.Migration(operator.Bundle(self.bundle), self.commands)

    def stage(self):
        self.migration.stage()
        self.commands.events.clear()
        self.commands.calls.clear()

    def cutover(self):
        self.stage()
        self.migration.cutover()

    def test_stage_rehearses_without_fencing_local_or_starting_target(self):
        self.migration.stage()
        self.assertTrue(self.migration.state['stage_complete'])
        self.assertEqual([event for event in self.commands.events if event.startswith('remote:')], ['remote:provision', 'remote:restore'])
        self.assertFalse(any(event.startswith(('disable:', 'bootout:', 'bootstrap:')) for event in self.commands.events))
        self.assertNotIn('export', self.commands.events)
        self.assertEqual(self.commands.loaded, set(operator.LABELS))
        self.assertEqual((self.local / 'deployment.json').read_bytes(), self.old_deployment)
        count = len(self.commands.calls)
        self.migration.stage()
        self.assertEqual(len(self.commands.calls), count)

    def test_cutover_fences_and_waits_before_export_then_orders_remote_activation(self):
        self.stage()
        self.migration.cutover()
        events = self.commands.events
        for label in operator.LABELS:
            self.assertLess(events.index('disable:' + label), events.index('bootout:' + label))
            self.assertLess(events.index('bootout:' + label), events.index('export'))
        expected = ['remote:preflight', 'disable:local.agentroom-github', 'export', 'upload', 'remote:provision', 'remote:restore', 'remote:start', 'remote:check', 'bootstrap:' + operator.TUNNEL, 'tunnel-listener', 'bootstrap:local.agentroom-sentry']
        indexes = [events.index(event) for event in expected]
        self.assertEqual(indexes, sorted(indexes))
        self.assertTrue(self.migration.state['cutover_complete'])
        self.assertFalse(self.migration.state.get('retired'))
        self.assertNotIn('docker-stop', events)
        self.assertFalse(any(event.startswith('serve-off:') for event in events))
        self.assertEqual(self.commands.loaded, {operator.TUNNEL, 'local.agentroom-sentry'})
        self.assertEqual(self.commands.disabled, {'local.agentroom', 'local.agentroom-github'})
        for args, kwargs in self.commands.calls:
            if Path(args[0]).name in ('ssh', 'scp'):
                self.assertIn('-oBatchMode=yes', args)
                self.assertIn('-oStrictHostKeyChecking=yes', args)
            if Path(args[0]).name == 'ssh' and shlex.split(args[-1])[0] == 'bash':
                self.assertIn('-t', args)
                self.assertTrue(kwargs['interactive'])
        self.assertNotIn('fixture-private-migration-content', repr(self.commands.calls))

    def test_cutover_requires_stage_and_final_export_proof(self):
        with self.assertRaises(operator.MigrationError):
            self.migration.cutover()
        self.assertEqual(self.commands.calls, [])
        self.stage()
        self.commands.final_proof = False
        with self.assertRaises(operator.MigrationError):
            self.migration.cutover()
        self.assertNotIn('upload', self.commands.events)
        self.assertNotIn('remote:start', self.commands.events)
        self.assertFalse(self.commands.loaded)

    def test_preflight_failure_preserves_running_writers_and_existing_checkpoint(self):
        self.stage()
        # Match recovery from the previous attempt: GitHub is already disabled,
        # while the application and Sentry are still serving/collecting locally.
        self.commands.loaded.discard('local.agentroom-github')
        self.commands.disabled.add('local.agentroom-github')
        self.migration.save('fencing', fence_started=True)
        checkpoint = self.migration.checkpoint.read_bytes()
        loaded, disabled = self.commands.loaded.copy(), self.commands.disabled.copy()
        self.commands.fail = 'remote:preflight'
        with self.assertRaises(operator.MigrationError):
            self.migration.cutover()
        self.assertEqual(self.commands.loaded, loaded)
        self.assertEqual(self.commands.disabled, disabled)
        self.assertEqual(self.migration.checkpoint.read_bytes(), checkpoint)
        self.assertEqual(self.commands.events, ['remote:preflight'])
        self.assertNotIn('export', self.commands.events)
        self.assertNotIn('remote:start', self.commands.events)

    def test_preflight_requires_durable_upload_binding_before_any_ssh_or_fence(self):
        self.stage()
        self.migration.state.pop('uploaded_manifest')
        self.migration.save()
        checkpoint = self.migration.checkpoint.read_bytes()
        with self.assertRaisesRegex(operator.MigrationError, 'previously uploaded manifest'):
            self.migration.cutover()
        self.assertEqual(self.commands.events, [])
        self.assertEqual(self.commands.loaded, set(operator.LABELS))
        self.assertEqual(self.commands.disabled, set())
        self.assertEqual(self.migration.checkpoint.read_bytes(), checkpoint)
        for invalid in (None, False, 'g' * 64, 'a' * 63):
            with self.subTest(invalid=invalid):
                self.migration.save(uploaded_manifest=invalid)
                with self.assertRaisesRegex(operator.MigrationError, 'previously uploaded manifest'):
                    self.migration.cutover()
                self.assertEqual(self.commands.events, [])
                self.assertEqual(self.commands.loaded, set(operator.LABELS))
                self.assertFalse(self.commands.disabled)

    def test_preflight_checks_uploaded_snapshot_in_operator_tty_not_unuploaded_final(self):
        self.stage()
        staged = self.migration.state['uploaded_manifest']
        write(self.bundle / 'snapshot.json', json.dumps({'schema': 1, 'kind': 'final', 'source_fenced': True}))
        seal(self.bundle)
        self.migration = operator.Migration(operator.Bundle(self.bundle), self.commands)
        self.assertNotEqual(self.migration.bundle.manifest_digest, staged)
        self.migration.remote('preflight')
        args, kwargs = self.commands.calls[-1]
        command = shlex.split(args[-1])
        self.assertEqual(command[:2], ['bash', '-c'])
        self.assertTrue(command[2].endswith('\nmain "$@"\n'))
        self.assertEqual(command[3:5], ['agentroom-preflight', 'preflight'])
        self.assertEqual(command[-1], staged)
        self.assertEqual(command[-2], self.migration.state['remote_bundle'])
        self.assertIn('-t', args)
        self.assertTrue(kwargs['interactive'])
        self.assertEqual(self.commands.events, ['remote:preflight'])
        self.assertEqual(self.commands.loaded, set(operator.LABELS))
        self.assertFalse(self.commands.disabled)

    def test_preflight_inline_program_is_one_quoted_argument_and_uploads_nothing(self):
        self.stage()
        program = "# fixture program with quotes\nprintf '%s' '$fixture; `not-executed`'\n"
        with patch.object(operator.Path, 'read_text', return_value=program):
            self.migration.remote('preflight')
        args, _ = self.commands.calls[-1]
        words = shlex.split(args[-1])
        self.assertEqual(words[2], program + '\nmain "$@"\n')
        self.assertEqual(words[3:], ['agentroom-preflight', 'preflight', self.migration.state['remote_bundle'], self.migration.state['uploaded_manifest']])
        self.assertEqual(self.commands.events, ['remote:preflight'])
        self.assertEqual(self.commands.loaded, set(operator.LABELS))
        self.assertFalse(self.commands.disabled)

    def test_preflight_oversized_inline_source_fails_before_ssh_or_local_changes(self):
        self.stage()
        checkpoint = self.migration.checkpoint.read_bytes()
        with patch.object(operator.Path, 'read_text', return_value='x' * (64 * 1024 + 1)):
            with self.assertRaisesRegex(operator.MigrationError, 'inline bound'):
                self.migration.cutover()
        self.assertEqual(self.commands.events, [])
        self.assertEqual(self.commands.loaded, set(operator.LABELS))
        self.assertFalse(self.commands.disabled)
        self.assertEqual(self.migration.checkpoint.read_bytes(), checkpoint)

    def test_uncertain_activation_never_restores_or_resumes_local_on_retry(self):
        self.stage()
        self.commands.fail = 'remote:start'
        with self.assertRaises(operator.MigrationError):
            self.migration.cutover()
        saved = operator.read_json(self.migration.checkpoint)
        self.assertTrue(saved['activation_possible'])
        self.assertFalse(self.commands.loaded)
        self.assertNotIn('bootstrap:local.agentroom-sentry', self.commands.events)
        self.commands.fail = None
        self.commands.events.clear()
        resumed = operator.Migration(operator.Bundle(self.bundle), self.commands)
        resumed.cutover()
        for forbidden in ('export', 'upload', 'remote:provision', 'remote:restore'):
            self.assertNotIn(forbidden, self.commands.events)
        self.assertEqual([event for event in self.commands.events if event.startswith('remote:')], ['remote:preflight', 'remote:start', 'remote:check'])
        self.assertTrue(resumed.state['cutover_complete'])

    def interrupt_final_provision(self):
        self.stage()
        staged = self.migration.state['uploaded_manifest']
        self.commands.fail = 'remote:provision'
        with self.assertRaises(operator.MigrationError):
            self.migration.cutover()
        self.commands.fail = None
        self.commands.events.clear()
        self.commands.calls.clear()
        self.migration = operator.Migration(operator.Bundle(self.bundle), self.commands)
        return staged

    def test_retry_after_final_upload_binds_both_snapshots_and_reuses_export(self):
        staged = self.interrupt_final_provision()
        final = self.migration.state['final_manifest']
        self.migration.cutover()
        self.assertEqual(shlex.split(self.commands.calls[0][0][-1])[-2:], [final, staged])
        self.assertEqual([event for event in self.commands.events if event.startswith('remote:')],
                         ['remote:preflight', 'remote:provision', 'remote:restore', 'remote:start', 'remote:check'])
        self.assertNotIn('export', self.commands.events)
        self.assertNotIn('upload', self.commands.events)
        self.assertTrue(self.migration.state['cutover_complete'])

    def test_retry_rejects_unrelated_upload_or_rehearsal_backup_before_ssh(self):
        self.interrupt_final_provision()
        saved = self.migration.state.copy()
        backup = Path(saved['stage_backup'])
        for changes in ({'uploaded_manifest': 'b' * 64}, {'stage_backup': str(self.bundle.parent)},
                        {'final_manifest': 'a' * 64}, {'fence_started': False}):
            with self.subTest(changes=changes):
                self.migration.state = saved | changes
                with self.assertRaises(operator.MigrationError):
                    self.migration.cutover()
                self.assertFalse(self.commands.calls)
        self.migration.state = saved
        write(backup / 'dumps/app.dump', 'altered backup')
        with self.assertRaises(operator.MigrationError):
            self.migration.cutover()
        self.assertFalse(self.commands.calls)

    def test_exported_snapshot_with_failed_upload_still_preflights_rehearsal(self):
        self.stage()
        staged = self.migration.state['uploaded_manifest']
        self.commands.fail = 'upload'
        with self.assertRaises(operator.MigrationError):
            self.migration.cutover()
        self.commands.fail = None
        self.commands.calls.clear()
        self.commands.events.clear()
        resumed = operator.Migration(operator.Bundle(self.bundle), self.commands)
        resumed.cutover()
        preflight = shlex.split(self.commands.calls[0][0][-1])
        self.assertEqual(preflight[4:], ['preflight', resumed.state['remote_bundle'], staged])
        self.assertNotIn('export', self.commands.events)
        self.assertIn('upload', self.commands.events)
        self.assertTrue(resumed.state['cutover_complete'])

    def test_retry_refuses_final_snapshot_if_source_writer_is_loaded(self):
        self.interrupt_final_provision()
        self.commands.loaded.add('local.agentroom')
        checkpoint = self.migration.checkpoint.read_bytes()
        with self.assertRaisesRegex(operator.MigrationError, 'local writer is loaded'):
            self.migration.cutover()
        self.assertNotIn('remote:provision', self.commands.events)
        self.assertEqual(self.migration.checkpoint.read_bytes(), checkpoint)

    def test_failed_fence_prevents_export_and_never_restarts_any_local_writer(self):
        self.stage()
        self.commands.fail = 'bootout:local.agentroom'
        with self.assertRaises(operator.MigrationError):
            self.migration.cutover()
        self.assertNotIn('export', self.commands.events)
        self.assertFalse(any(event.startswith('bootstrap:') for event in self.commands.events))
        self.assertTrue(self.migration.state['fence_started'])

    def test_cutover_waits_for_asynchronous_unload_before_final_export(self):
        self.stage()
        original = self.commands.run
        pending = {}
        completed = {}

        def delayed(args, **kwargs):
            if Path(args[0]).name == 'launchctl':
                action, label = args[1], args[2].split('/', 2)[-1]
                if action == 'print' and label in pending:
                    pending[label] += 1
                    if pending[label] >= 3:
                        self.commands.loaded.discard(label)
                        completed[label] = pending.pop(label)
                result = original(args, **kwargs)
                if action == 'bootout':
                    self.commands.loaded.add(label)
                    pending[label] = 0
                return result
            return original(args, **kwargs)

        with patch.object(self.commands, 'run', side_effect=delayed), \
             patch.object(operator.time, 'sleep'):
            self.migration.cutover()
        self.assertTrue(self.migration.state['cutover_complete'])
        self.assertTrue(all(completed[label] >= 3 for label in operator.LABELS))
        # The fixture exporter independently asserts every writer is unloaded
        # and persistently disabled before accepting this final snapshot.
        self.assertIn('export', self.commands.events)

    def test_unload_timeout_keeps_fence_and_never_exports(self):
        self.stage()
        original = self.commands.run

        def never_unloads(args, **kwargs):
            result = original(args, **kwargs)
            if Path(args[0]).name == 'launchctl' and args[1] == 'bootout':
                self.commands.loaded.add(args[2].split('/', 2)[-1])
            return result

        with patch.object(self.commands, 'run', side_effect=never_unloads), \
             patch.object(operator.time, 'sleep'), \
             patch.object(operator.time, 'monotonic', side_effect=[0, 0, 31]):
            with self.assertRaisesRegex(operator.MigrationError, 'shutdown timeout'):
                self.migration.cutover()
        self.assertTrue(self.migration.state['fence_started'])
        self.assertNotIn('export', self.commands.events)
        self.assertNotIn('remote:start', self.commands.events)
        self.assertFalse(any(event.startswith('bootstrap:') for event in self.commands.events))

    def test_failed_remote_check_keeps_publishers_and_hosting_stopped(self):
        self.stage()
        self.commands.fail = 'remote:check'
        with self.assertRaises(operator.MigrationError):
            self.migration.cutover()
        self.assertTrue(self.migration.state['activation_possible'])
        self.assertFalse(self.migration.state['remote_check_passed'])
        self.assertFalse(self.commands.loaded)
        self.assertFalse(any(event.startswith('bootstrap:') for event in self.commands.events))
        self.assertEqual((self.local / 'deployment.json').read_bytes(), self.old_deployment)

    def test_failed_final_restore_keeps_fence_and_preserves_both_snapshot_sets(self):
        self.stage()
        self.commands.fail = 'remote:restore'
        with self.assertRaises(operator.MigrationError):
            self.migration.cutover()
        self.assertFalse(self.migration.state.get('activation_possible'))
        self.assertFalse(self.commands.loaded)
        self.assertNotIn('remote:start', self.commands.events)
        self.assertTrue(Path(self.migration.state['stage_backup']).is_dir())
        self.assertTrue(Path(self.migration.state['final_backup']).is_dir())
        self.assertTrue((self.bundle / 'github/state/queue.sqlite3').is_file())

    def test_tunnel_persists_on_login_while_old_hosting_and_github_remain_disabled(self):
        self.cutover()
        link = self.library / (operator.TUNNEL + '.plist')
        self.assertTrue(link.is_symlink())
        self.assertEqual(link.resolve(), self.migration.directory / 'local.agentroom-tunnel.plist')
        payload = plistlib.loads(link.read_bytes())
        for flag in ('-NT', '-oExitOnForwardFailure=yes', '-oServerAliveInterval=30', '-oServerAliveCountMax=3', '-oControlMaster=no', '-oControlPath=none', '-L127.0.0.1:54491:127.0.0.1:54491'):
            self.assertIn(flag, payload['ProgramArguments'])
        self.assertEqual(link.resolve().stat().st_mode & 0o077, 0)
        login_loaded = {plistlib.loads(path.read_bytes())['Label'] for path in self.library.glob('*.plist') if plistlib.loads(path.read_bytes())['Label'] not in self.commands.disabled}
        self.assertEqual(login_loaded, {operator.TUNNEL, 'local.agentroom-sentry'})

    def test_foreign_tunnel_link_is_not_replaced_and_sentry_remains_stopped(self):
        self.stage()
        foreign = self.library / (operator.TUNNEL + '.plist')
        foreign.symlink_to(self.local / 'local.agentroom.plist')
        with self.assertRaises(operator.MigrationError):
            self.migration.cutover()
        self.assertEqual(os.readlink(foreign), str(self.local / 'local.agentroom.plist'))
        self.assertNotIn('local.agentroom-sentry', self.commands.loaded)

    def test_retirement_requires_explicit_browser_acceptance_and_prior_checks(self):
        self.cutover()
        self.commands.events.clear()
        with self.assertRaises(operator.MigrationError):
            self.migration.retire(None)
        self.assertEqual(self.commands.events, [])
        self.migration.save(remote_check_passed=False)
        with self.assertRaises(operator.MigrationError):
            self.migration.retire('REMOTE_BROWSER_ACCEPTED')
        self.assertNotIn('docker-stop', self.commands.events)

    def test_retirement_rechecks_remote_before_stopping_any_local_database(self):
        self.cutover()
        self.commands.events.clear()
        self.commands.fail = 'remote:check'
        with self.assertRaises(operator.MigrationError):
            self.migration.retire('REMOTE_BROWSER_ACCEPTED')
        self.assertEqual(self.commands.events, ['remote:check'])
        self.assertFalse(self.migration.state['remote_check_passed'])
        self.assertEqual(self.migration.state['phase'], 'cutover-complete')

    def test_standalone_check_preserves_cutover_phase_on_transient_failure_and_recovery(self):
        self.cutover()
        self.commands.events.clear()
        self.commands.fail = 'remote:check'
        with self.assertRaises(operator.MigrationError):
            self.migration.check()
        self.assertEqual(self.migration.state['phase'], 'cutover-complete')
        self.assertFalse(self.migration.state['remote_check_passed'])
        self.commands.fail = None
        self.migration.check()
        self.assertEqual(self.migration.state['phase'], 'cutover-complete')
        self.assertTrue(self.migration.state['remote_check_passed'])

    def test_retire_only_stops_own_databases_and_two_owned_routes(self):
        self.cutover()
        self.commands.events.clear()
        self.commands.calls.clear()
        with patch.object(operator.shutil, 'which', return_value='/fixture/bin/tailscale'):
            self.migration.retire('REMOTE_BROWSER_ACCEPTED')
        self.assertLess(self.commands.events.index('remote:check'), self.commands.events.index('docker-stop'))
        self.assertEqual([event for event in self.commands.events if event.startswith('serve-off:')], ['serve-off:4445', 'serve-off:4446'])
        docker = next(args for args, _ in self.commands.calls if Path(args[0]).name == 'docker')
        self.assertEqual(docker[-3:], ['stop', 'dex', 'postgres'])
        self.assertNotIn('down', docker)
        self.assertEqual(self.commands.loaded, {operator.TUNNEL, 'local.agentroom-sentry'})
        self.assertTrue(self.bundle.exists())
        self.assertTrue((self.bundle / 'github/state/queue.sqlite3').exists())
        self.assertTrue((self.migration.directory / 'snapshots/retire-serve.json').exists())
        record = json.loads((self.local / 'deployment.json').read_text())
        self.assertTrue(record['local']['retired'])
        self.assertEqual(record['phase'], 'remote-browser-accepted-local-retired')
        self.assertTrue(self.migration.state['retired'])

    def test_retire_refuses_neighbor_routes_before_stopping_local_databases(self):
        self.cutover()
        self.commands.events.clear()
        self.commands.serve['Web']['local.fixture.ts.net:4445']['Handlers']['/neighbor'] = {'Proxy': 'http://127.0.0.1:9000'}
        with patch.object(operator.shutil, 'which', return_value='/fixture/bin/tailscale'):
            with self.assertRaises(operator.MigrationError):
                self.migration.retire('REMOTE_BROWSER_ACCEPTED')
        self.assertNotIn('docker-stop', self.commands.events)
        self.assertFalse(any(event.startswith('serve-off:') for event in self.commands.events))

    def test_handoff_preserves_original_provenance_and_explicit_remote_authority(self):
        self.cutover()
        snapshot = self.migration.directory / 'snapshots/deployment-before-migration.json'
        self.assertEqual(snapshot.read_bytes(), self.old_deployment)
        record = json.loads((self.local / 'deployment.json').read_text())
        self.assertEqual(record['kind'], 'shared-host-migration')
        self.assertEqual(record['authoritative_checkpoint'], str(self.migration.checkpoint))
        self.assertEqual(record['active']['public_url'], self.meta['public_url'])
        self.assertFalse(record['local']['retired'])
        self.assertEqual(record['phase'], 'remote-checked-browser-acceptance-pending')
        self.migration.cutover()
        self.assertEqual(snapshot.read_bytes(), self.old_deployment)

    def test_failed_handoff_is_repaired_on_completed_cutover_and_retire_retries(self):
        self.stage()
        with patch.object(self.migration, 'deployment_handoff', side_effect=operator.MigrationError('fixture handoff failure')):
            with self.assertRaises(operator.MigrationError):
                self.migration.cutover()
        self.assertTrue(self.migration.state['cutover_complete'])
        self.commands.events.clear()
        self.migration.cutover()
        self.assertEqual(self.commands.events, ['remote:check'])
        self.assertEqual(json.loads((self.local / 'deployment.json').read_text())['kind'], 'shared-host-migration')
        with patch.object(operator.shutil, 'which', return_value='/fixture/bin/tailscale'), patch.object(self.migration, 'deployment_handoff', side_effect=operator.MigrationError('fixture handoff failure')):
            with self.assertRaises(operator.MigrationError):
                self.migration.retire('REMOTE_BROWSER_ACCEPTED')
        self.assertTrue(self.migration.state['retired'])
        self.commands.events.clear()
        self.migration.retire('REMOTE_BROWSER_ACCEPTED')
        self.assertNotIn('docker-stop', self.commands.events)
        self.assertFalse(any(event.startswith('serve-off:') for event in self.commands.events))
        self.assertTrue(json.loads((self.local / 'deployment.json').read_text())['local']['retired'])

    def test_bundle_tamper_links_and_unknown_files_fail_before_commands(self):
        for change in ('tamper', 'link', 'extra'):
            with self.subTest(change=change):
                path = self.bundle / 'dumps/app.dump'
                if change == 'tamper':
                    path.write_text('changed')
                elif change == 'link':
                    path.unlink()
                    path.symlink_to(self.bundle / 'dumps/identity.dump')
                else:
                    write(self.bundle / 'extra', 'unmanifested')
                with self.assertRaises(operator.MigrationError):
                    operator.Bundle(self.bundle)
                self.assertEqual(self.commands.calls, [])
                if path.is_symlink():
                    path.unlink()
                write(path, 'fixture-private-migration-content')
                if (self.bundle / 'extra').exists():
                    (self.bundle / 'extra').unlink()

    def test_checkpoint_cannot_be_rebound_to_another_host(self):
        self.stage()
        self.control['ssh_host'] = 'other-host'
        write(self.bundle / 'operator.json', json.dumps(self.control))
        seal(self.bundle)
        with self.assertRaises(operator.MigrationError):
            operator.Migration(operator.Bundle(self.bundle), self.commands)

    def test_unsafe_ssh_alias_and_mutated_incoming_checkpoint_are_rejected(self):
        self.control['ssh_host'] = 'host; echo fixture-private'
        write(self.bundle / 'operator.json', json.dumps(self.control))
        seal(self.bundle)
        with self.assertRaises(operator.MigrationError):
            operator.Bundle(self.bundle)
        self.migration.save(remote_bundle='/etc/unrelated')
        with self.assertRaises(operator.MigrationError):
            self.migration.remote_bundle()
        self.assertEqual(self.commands.calls, [])

    def test_private_checkpoint_and_single_writer_lock(self):
        self.migration.save('test')
        self.assertEqual(self.migration.checkpoint.stat().st_mode & 0o077, 0)
        with operator.process_lock(self.migration.directory):
            with self.assertRaises(operator.MigrationError):
                with operator.process_lock(self.migration.directory):
                    self.fail('second writer acquired the lock')

    def test_absent_bundle_is_not_created_and_import_executes_nothing(self):
        absent = self.root / 'missing'
        with self.assertRaises(operator.MigrationError):
            operator.Bundle(absent)
        self.assertFalse(absent.exists())
        with patch.object(operator.subprocess, 'run') as command:
            spec = importlib.util.spec_from_file_location('migration_import_test', SOURCE)
            spec.loader.exec_module(importlib.util.module_from_spec(spec))
        command.assert_not_called()


class CommandLineTests(unittest.TestCase):
    def invoke(self, tty, bundle_error=None):
        import contextlib
        import io
        from types import SimpleNamespace
        errors = io.StringIO()
        with patch.object(operator.sys, 'argv', ['run-migration.py', '--bundle', '/fixture/bundle', 'stage']), \
             patch.object(operator.sys, 'platform', 'darwin'), \
             patch.object(operator.os, 'getuid', return_value=operator.UID), \
             patch.object(operator.sys, 'stdin', SimpleNamespace(isatty=lambda: tty)), \
             patch.object(operator, 'Bundle', side_effect=bundle_error) as bundle, \
             contextlib.redirect_stderr(errors):
            result = operator.main()
        return result, errors.getvalue(), bundle

    def test_noninteractive_shell_explains_terminal_requirement_before_loading_bundle(self):
        result, errors, bundle = self.invoke(False)
        self.assertEqual(result, 1)
        self.assertIn('interactive terminal', errors)
        bundle.assert_not_called()

    def test_unexpected_error_details_remain_private(self):
        result, errors, _ = self.invoke(True, OSError('fixture-private-secret'))
        self.assertEqual(result, 1)
        self.assertNotIn('fixture-private-secret', errors)
        self.assertIn('private checkpoint/log', errors)


if __name__ == '__main__':
    unittest.main()
