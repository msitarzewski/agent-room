"""Collector contract tests; all records, tokens, and transports are test fixtures."""
from __future__ import annotations

import contextlib
import dataclasses
import importlib.util
import io
import json
from pathlib import Path
import stat
import sys
import tempfile
import types
import unittest
from unittest.mock import Mock, patch
import urllib.error


SOURCE = Path(__file__).resolve().parents[2] / "adapters/sentry/agentroom_sentry.py"
SPEC = importlib.util.spec_from_file_location("agentroom_sentry", SOURCE)
connector = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = connector
SPEC.loader.exec_module(connector)


def task(title="Actual fixture task", ident="hash", source="memory-bank/tasks/checklist.md", status="Upcoming"):
    return types.SimpleNamespace(source=source, ident=ident, title=title, status=types.SimpleNamespace(value=status), heading="Next", detail=(("p", "Fixture evidence description"),))


def report(items=None, kept=None):
    return {"name": "Fixture project", "status": "Active", "reason": "Touched recently", "scan_status": "partial" if kept else "complete", "items": [task()] if items is None else items, "kept": kept or [], "scan_error": "Retained records" if kept else ""}


class CollectorTest(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix="agentroom-collector-test-")
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name).resolve()
        self.project = self.root / "project"
        self.project.mkdir()
        self.token = self.root / "token"
        self.token.write_text("fixture-token")
        self.token.chmod(0o600)
        self.mapping = connector.ProjectMapping(self.project, "project-test", self.token)
        self.config = connector.Config(self.root / "sentry", self.root / "sentry.toml", self.root / "state", "fixture-instance", "http://127.0.0.1:8082/api/v1/ingest", (self.mapping,))
        self.state = connector.State(self.config.state_dir)
        self.addCleanup(lambda: self.state.close() if self.state else None)
        self.scan = {"projects": {str(self.project): report()}, "unreadable": [], "skipped": []}
        self.reader = Mock()
        self.reader.collect.return_value = self.scan
        self.sender = Mock(return_value=None)
        self.collector = connector.Collector(self.config, self.state, self.reader, self.sender)
        self.now = 1792015200.0

    def queue(self):
        return [dict(row) for row in self.state.db.execute("SELECT * FROM queue ORDER BY id")]

    def test_collects_once_and_preserves_pilot_identity_and_native_source_status(self):
        self.collector.collect(self.now)
        self.reader.collect.assert_called_once_with(self.now)
        events = [json.loads(row["body"]) for row in self.queue()]
        self.assertEqual([event["type"] for event in events], ["project.observed", "task.observed"])
        self.assertEqual(events[1]["payload"]["external_id"], "memory-bank/tasks/checklist.md#hash")
        self.assertEqual(events[1]["payload"]["status"], "Upcoming")
        self.assertEqual(events[0]["payload"]["collected_at"], events[0]["occurred_at"])
        self.assertGreater(events[0]["source_sequence"], 10**15)
        self.assertGreater(events[1]["source_sequence"], events[0]["source_sequence"])
        self.assertNotIn("resource", events[1]["payload"])

    def test_deduplicates_unchanged_tasks_but_publishes_every_scan_receipt(self):
        self.collector.collect(self.now)
        self.collector.collect(self.now + 300)
        events = [json.loads(row["body"]) for row in self.queue()]
        self.assertEqual(sum(event["type"] == "task.observed" for event in events), 1)
        self.assertEqual(sum(event["type"] == "project.observed" for event in events), 2)
        self.scan["projects"][str(self.project)]["items"] = [task(status="Done")]
        self.collector.collect(self.now + 600)
        self.assertEqual(sum(json.loads(row["body"])["type"] == "task.observed" for row in self.queue()), 2)

    def test_restart_retries_identical_persisted_events_after_uncertain_delivery(self):
        self.collector.collect(self.now)
        self.sender.return_value = "http_503"
        self.collector.deliver(self.now)
        original = self.sender.call_args.args[2]
        self.assertEqual(len(self.queue()), 2)
        self.assertIsNone(self.state.status()["projects"][0]["last_delivery"])
        self.state.close()
        self.state = connector.State(self.config.state_dir)
        resumed = connector.Collector(self.config, self.state, self.reader, self.sender)
        self.sender.return_value = None
        resumed.deliver(self.now + 6)
        self.assertEqual(self.sender.call_args.args[2], original)
        self.assertEqual(self.queue(), [])
        self.assertEqual(self.state.status()["projects"][0]["last_delivery"], connector.timestamp(self.now + 6))

    def test_sequence_survives_clock_rollback_and_adopts_existing_large_watermark(self):
        with self.state.db:
            self.state.set_meta("sequence", 2_000_000_000_000_000_000)
        with patch.object(connector.time, "time_ns", return_value=100):
            self.collector.collect(self.now)
        sequences = [json.loads(row["body"])["source_sequence"] for row in self.queue()]
        self.assertEqual(sequences, [2_000_000_000_000_000_001, 2_000_000_000_000_000_002])

    def test_offline_collection_persists_new_work_without_repeated_target_timeouts(self):
        self.sender.return_value = "transport_unavailable"
        self.collector.collect(self.now)
        self.collector.deliver(self.now)
        self.collector.collect(self.now + 1)
        self.collector.deliver(self.now + 1)
        self.assertEqual(self.sender.call_count, 1)
        self.assertEqual(len(self.queue()), 3)
        self.assertEqual(self.state.status()["transport_error"], "transport_unavailable")

    def test_project_authorization_failure_does_not_block_another_project(self):
        other = connector.ProjectMapping(self.root / "other", "other-project", self.token)
        config = dataclasses.replace(self.config, projects=(self.mapping, other))
        self.scan["projects"][str(other.path)] = report([])
        collector = connector.Collector(config, self.state, self.reader, self.sender)
        self.sender.side_effect = ["http_403", None]
        collector.collect(self.now)
        collector.deliver(self.now)
        self.assertEqual(self.sender.call_count, 2)
        self.assertTrue(all(row["project_id"] == self.mapping.project_id for row in self.queue()))
        status = {item["project_id"]: item for item in self.state.status()["projects"]}
        self.assertEqual(status[self.mapping.project_id]["delivery_error"], "http_403")
        self.assertIsNotNone(status[other.project_id]["last_delivery"])

    def test_window_kept_and_unreadable_sources_never_delete_or_recomplete_tasks(self):
        self.collector.collect(self.now)
        kept = [(str(self.project), "memory-bank/tasks/old.md", None)]
        self.scan["projects"][str(self.project)] = report([], kept)
        self.collector.collect(self.now + 300)
        self.assertEqual(self.state.status()["projects"][0]["scan_status"], "partial")
        self.assertEqual(self.state.status()["projects"][0]["kept"], [list(kept[0])])
        del self.scan["projects"][str(self.project)]
        self.scan["unreadable"] = [str(self.project)]
        self.collector.collect(self.now + 600)
        self.assertEqual(self.state.status()["projects"][0]["scan_status"], "unreadable")
        self.assertEqual(self.state.status()["unreadable"], [str(self.project)])
        self.assertEqual(sum(json.loads(row["body"])["type"] == "task.observed" for row in self.queue()), 1)
        self.assertEqual(self.state.status()["projects"][0]["last_success"], connector.timestamp(self.now))

    def test_unknown_projects_require_explicit_provisioning_and_never_publish(self):
        unknown = str(self.root / "unknown")
        self.scan["projects"][unknown] = report([task("Unknown fixture task")])
        self.collector.collect(self.now)
        self.assertEqual(self.state.status()["needs_provisioning"], [{"path": unknown, "name": "Fixture project"}])
        self.assertEqual({row["project_id"] for row in self.queue()}, {self.mapping.project_id})

    def test_global_scan_failure_retains_last_success_and_does_not_claim_freshness(self):
        self.collector.collect(self.now)
        self.reader.collect.side_effect = PermissionError("fixture secret must not reach status")
        self.collector.collect(self.now + 300)
        status = self.state.status()
        self.assertEqual(status["collection_error"], "PermissionError")
        self.assertEqual(status["last_collection"], connector.timestamp(self.now))
        self.assertNotIn("fixture secret", json.dumps(status))

    def test_only_202_acknowledges_and_invalid_batches_are_retained_for_review(self):
        self.collector.collect(self.now)
        self.sender.return_value = "http_200"
        self.collector.deliver(self.now)
        self.assertEqual(len(self.queue()), 2)
        self.sender.return_value = "http_400"
        self.collector.deliver(self.now + 6)
        self.assertEqual(self.state.status()["quarantined"], 2)
        self.assertEqual(self.state.status()["pending"], 0)
        self.assertEqual(len(self.queue()), 2)

    def test_old_observations_are_quarantined_without_retimestamps_or_discard(self):
        self.collector.collect(self.now)
        original = [row["body"] for row in self.queue()]
        self.collector.deliver(self.now + 31 * 86400)
        self.sender.assert_not_called()
        self.assertEqual([row["body"] for row in self.queue()], original)
        self.assertEqual(self.state.status()["quarantined"], 2)
        self.assertTrue(all(row["error"] == "observation_older_than_30_days_requires_review" for row in self.queue()))

    def test_batches_are_bounded_and_remaining_rows_stay_durable(self):
        config = dataclasses.replace(self.config, batch_size=2)
        collector = connector.Collector(config, self.state, self.reader, self.sender)
        self.scan["projects"][str(self.project)] = report([task(ident=str(index)) for index in range(5)])
        collector.collect(self.now)
        collector.deliver(self.now)
        self.assertEqual(len(json.loads(self.sender.call_args.args[2])["events"]), 2)
        self.assertEqual(len(self.queue()), 4)

    def test_state_and_lock_are_private_and_source_instance_is_bound(self):
        self.assertEqual(stat.S_IMODE(self.config.state_dir.stat().st_mode), 0o700)
        self.assertEqual(stat.S_IMODE(self.state.path.stat().st_mode), 0o600)
        with connector.process_lock(self.config.state_dir):
            with self.assertRaises(connector.ConnectorError):
                with connector.process_lock(self.config.state_dir):
                    self.fail("Second process unexpectedly acquired the lock")
        with self.assertRaises(connector.ConnectorError):
            connector.Collector(dataclasses.replace(self.config, instance_id="another"), self.state, self.reader)
        self.assertEqual(connector.read_status(self.config)["pending"], 0)

    def test_project_to_resolved_path_binding_cannot_change_across_restart(self):
        self.state.close()
        self.state = connector.State(self.config.state_dir)
        changed = dataclasses.replace(self.mapping, path=self.root / "different-repository")
        with self.assertRaisesRegex(connector.ConnectorError, "explicit state migration"):
            connector.Collector(dataclasses.replace(self.config, projects=(changed,)), self.state, self.reader)
        renamed = dataclasses.replace(self.mapping, project_id="different-project")
        with self.assertRaisesRegex(connector.ConnectorError, "explicit state migration"):
            connector.Collector(dataclasses.replace(self.config, projects=(renamed,)), self.state, self.reader)
        connector.Collector(self.config, self.state, self.reader)

    def test_long_source_descriptions_are_explicitly_marked_as_excerpts(self):
        long = task()
        long.detail = (("p", "long source " * 2000),)
        self.scan["projects"][str(self.project)] = report([long])
        self.collector.collect(self.now)
        description = json.loads(self.queue()[1]["body"])["payload"]["description"]
        self.assertEqual(len(description), 16384)
        self.assertIn("Source excerpt truncated", description)
        self.assertTrue(description.endswith("complete record.]"))

    def test_http_publisher_never_follows_redirects_or_prints_token_or_response_body(self):
        response = Mock()
        response.status = 202
        response.__enter__ = Mock(return_value=response)
        response.__exit__ = Mock(return_value=False)
        opener = Mock()
        opener.open.return_value = response
        with patch.object(connector.urllib.request, "build_opener", return_value=opener) as build:
            self.assertIsNone(connector.publish(self.config, self.mapping, b'{"events":[]}'))
        self.assertIsInstance(build.call_args.args[1], connector.NoRedirect)
        request = opener.open.call_args.args[0]
        self.assertEqual(request.get_header("Authorization"), "Bearer fixture-token")
        self.assertIn("project_id=project-test", request.full_url)
        self.assertIsNone(connector.NoRedirect().redirect_request(None, None, 302, None, None, "https://outside.invalid"))
        self.token.chmod(0o644)
        self.assertEqual(connector.publish(self.config, self.mapping, b"{}"), "private_token_unavailable")

    def test_token_and_transport_errors_are_fixed_nonsecret_codes(self):
        opener = Mock()
        opener.open.side_effect = urllib.error.HTTPError("https://private.invalid", 401, "fixture secret", {}, None)
        with patch.object(connector.urllib.request, "build_opener", return_value=opener):
            self.assertEqual(connector.publish(self.config, self.mapping, b"{}"), "http_401")
            opener.open.side_effect = urllib.error.URLError("fixture secret")
            self.assertEqual(connector.publish(self.config, self.mapping, b"{}"), "transport_unavailable")
        self.token.write_text("fixture-token\ninjected")
        self.assertEqual(connector.publish(self.config, self.mapping, b"{}"), "invalid_token_file")

    def test_cli_once_and_status_report_delivery_without_task_or_token_contents(self):
        self.state.close()
        self.state = None
        path = self.root / "connector.toml"
        path.write_text(f'''[connector]
sentry_checkout = {json.dumps(str(self.config.sentry_checkout))}
sentry_config = {json.dumps(str(self.config.sentry_config))}
state_dir = {json.dumps(str(self.config.state_dir))}
instance_id = "fixture-instance"
ingest_url = "http://127.0.0.1:8082/api/v1/ingest"
[[projects]]
path = {json.dumps(str(self.project))}
project_id = "project-test"
token_file = {json.dumps(str(self.token))}
''')
        for command in ("once", "status"):
            output = io.StringIO()
            with patch.object(sys, "argv", ["connector", "--config", str(path), command]), patch.object(connector, "SentryReader", return_value=self.reader), patch.object(connector, "publish", return_value=None), patch.object(connector.signal, "signal"), contextlib.redirect_stdout(output):
                self.assertEqual(connector.main(), 0)
            status = json.loads(output.getvalue())
            self.assertEqual(status["pending"], 0)
            self.assertIsNotNone(status["projects"][0]["last_delivery"])
            self.assertNotIn("fixture-token", output.getvalue())
            self.assertNotIn("Actual fixture task", output.getvalue())
        stopped = Mock()
        stopped.is_set.side_effect = [False, True]
        with patch.object(sys, "argv", ["connector", "--config", str(path), "watch"]), patch.object(connector, "SentryReader", return_value=self.reader), patch.object(connector, "publish", return_value=None), patch.object(connector.signal, "signal"), patch.object(connector.threading, "Event", return_value=stopped):
            self.assertEqual(connector.main(), 0)
        stopped.wait.assert_called_once()
        output = io.StringIO()
        with patch.object(sys, "argv", ["connector", "--config", str(path), "once"]), patch.object(connector, "load_config", return_value=dataclasses.replace(self.config, batch_size=1)), patch.object(connector, "SentryReader", return_value=self.reader), patch.object(connector, "publish", return_value="http_503"), patch.object(connector.signal, "signal"), contextlib.redirect_stdout(output):
            self.assertEqual(connector.main(), 1)
        # A separate state tests a successful but deliberately bounded delivery pass.
        bounded = dataclasses.replace(self.config, state_dir=self.root / "bounded-state", batch_size=1)
        with patch.object(sys, "argv", ["connector", "--config", str(path), "once"]), patch.object(connector, "load_config", return_value=bounded), patch.object(connector, "SentryReader", return_value=self.reader), patch.object(connector, "publish", return_value=None), patch.object(connector.signal, "signal"), contextlib.redirect_stdout(output):
            self.assertEqual(connector.main(), 2)

    def test_transaction_failure_does_not_advance_fingerprint_or_collection_checkpoint(self):
        original = self.state.enqueue
        def fail_task(config, mapping, event_type, payload, now, deduplicate):
            original(config, mapping, event_type, payload, now, deduplicate)
            if event_type == "task.observed":
                raise OSError("Fixture database write failed")
        with patch.object(self.state, "enqueue", side_effect=fail_task):
            with self.assertRaises(OSError):
                self.collector.collect(self.now)
        self.assertEqual(self.queue(), [])
        self.assertIsNone(self.state.meta("last_collection"))
        self.assertEqual(self.state.db.execute("SELECT count(*) FROM observations").fetchone()[0], 0)
        self.collector.collect(self.now + 1)
        self.assertEqual(len(self.queue()), 2)

    def test_oversize_single_event_is_quarantined_and_private_state_rejects_world_access(self):
        self.collector.collect(self.now)
        config = dataclasses.replace(self.config, max_batch_bytes=1)
        connector.Collector(config, self.state, self.reader, self.sender).deliver(self.now)
        self.assertEqual(self.state.status()["quarantined"], 1)
        self.sender.assert_not_called()
        unsafe = self.root / "unsafe"
        unsafe.mkdir(mode=0o755)
        unsafe.chmod(0o755)
        with self.assertRaises(connector.ConnectorError):
            connector.private_directory(unsafe)


class ReaderTest(unittest.TestCase):
    def test_reuses_scanner_once_and_preserves_kept_and_partial_project_failures(self):
        reader = connector.SentryReader.__new__(connector.SentryReader)
        root = Path("/fixture/projects")
        paths = [root / "good", root / "unreadable", root / "upstream", root / "bad-parser"]
        good = types.SimpleNamespace(name="good", upstream=False, has_memory_bank=True)
        upstream = types.SimpleNamespace(name="upstream", upstream=True)
        reader.config = types.SimpleNamespace(roots=(root,), exclude=frozenset(), owners=frozenset(), thresholds=object(), tasks_window_days=90)
        reader.scanner = types.SimpleNamespace(discover=Mock(return_value=(paths, [], [])), scan=Mock(side_effect=[good, PermissionError(), upstream, ValueError("private fixture content")]), read_tasks=Mock(return_value=types.SimpleNamespace(items=[task()], kept=[(paths[0], "memory-bank/tasks/old.md", None)])))
        reader.rules = types.SimpleNamespace(classify=Mock(return_value=(types.SimpleNamespace(value="Active"), "Recent source")))
        result = reader.collect(1792015200)
        reader.scanner.discover.assert_called_once()
        self.assertEqual(reader.scanner.scan.call_count, 4)
        reader.scanner.read_tasks.assert_called_once_with(paths[0], 1792015200, 90)
        self.assertEqual(result["projects"][str(paths[0])]["scan_status"], "partial")
        self.assertEqual(result["unreadable"], [str(paths[1])])
        self.assertNotIn(str(paths[2]), result["projects"])
        self.assertEqual(result["projects"][str(paths[3])]["scan_error"], "ValueError")


class ConfigTest(unittest.TestCase):
    def test_config_requires_private_transport_explicit_unique_mappings_and_bounded_limits(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / "connector.toml"
            template = (SOURCE.parents[2] / "deploy/config/sentry-connector.toml.example").read_text()
            path.write_text(template)
            config = connector.load_config(path)
            self.assertEqual(config.poll_seconds, 300)
            self.assertEqual(len(config.projects), 1)
            for original, replacement in [("http://127.0.0.1:8082", "http://outside.invalid:8082"), ("http://127.0.0.1:8082", "https://user:password@outside.invalid"), ("/api/v1/ingest", "/api/v1/ingest?token=private"), ("batch_size = 50", "batch_size = 1000")]:
                path.write_text(template.replace(original, replacement))
                with self.assertRaises(connector.ConnectorError):
                    connector.load_config(path)

    def test_status_without_database_is_read_only_and_private_paths_reject_symlinks(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            config = connector.Config(root, root, root / "absent", "fixture", "http://127.0.0.1/api/v1/ingest", ())
            self.assertFalse(connector.read_status(config)["initialized"])
            self.assertFalse(config.state_dir.exists())
            linked = root / "link"
            linked.symlink_to(root, target_is_directory=True)
            with self.assertRaises(connector.ConnectorError):
                connector.private_directory(linked)


if __name__ == "__main__":
    unittest.main()
