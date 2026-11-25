import importlib.util
import contextlib
import io
import json
import os
from pathlib import Path
import sqlite3
import tarfile
import tempfile
import unittest
from unittest import mock


ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("prepare_migration", ROOT / "deploy/prepare-migration.py")
migration = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(migration)


class MigrationTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)

    def test_origin_requires_private_https_exact_port_and_no_url_payload(self):
        self.assertEqual(migration.origin("https://node.tail-example.ts.net:4445/"),
                         "https://node.tail-example.ts.net:4445")
        for value in ["http://node.tail-example.ts.net:4445", "https://example.com:4445",
                      "https://node.tail-example.ts.net:4446", "https://node.tail-example.ts.net:4445/path",
                      "https://user@node.tail-example.ts.net:4445", "https://node.tail-example.ts.net:4445?x=1",
                      "https://node.tail-example.ts.net:4445#x", "https://node.tail-example.ts.net:bad"]:
            with self.subTest(value=value), self.assertRaises((migration.MigrationError, ValueError)):
                migration.origin(value)

    def test_config_parser_retains_literal_shell_syntax_without_executing(self):
        path = self.root / "app.conf"
        path.write_text('AGENTROOM_PUBLIC_URL="$(touch should-not-exist)"\nAGENTROOM_DEV=false\n')
        self.assertEqual(migration.config_values(path)["AGENTROOM_PUBLIC_URL"], "$(touch should-not-exist)")
        self.assertFalse((self.root / "should-not-exist").exists())

    def test_config_parser_rejects_duplicates_and_non_configuration_keys(self):
        path = self.root / "app.conf"
        for value in ["AGENTROOM_DEV=true\nAGENTROOM_DEV=false\n", "OTHER=value\n", "missing assignment\n"]:
            path.write_text(value)
            with self.assertRaises(migration.MigrationError):
                migration.config_values(path)

    def test_sql_identifier_cannot_inject_commands(self):
        self.assertEqual(migration.identifier("agentroom_identity"), "agentroom_identity")
        for value in ["a;DROP DATABASE postgres", "a-b", 'a"', "A", "a" * 64, ""]:
            with self.subTest(value=value), self.assertRaises(migration.MigrationError):
                migration.identifier(value)

    def test_sql_literal_quotes_password_and_rejects_control_input(self):
        self.assertEqual(migration.literal("it's private"), "'it''s private'")
        for value in ["a\x00b", "a\nb", "a\rb"]:
            with self.assertRaises(migration.MigrationError):
                migration.literal(value)

    def test_identity_subject_and_bcrypt_scalar_are_preserved(self):
        identity = 'staticPasswords:\n  - email: operator@example.invalid\n    userID: "original-subject"\n    hash: "$2b$fixture"\n'
        self.assertEqual(migration.scalar(identity, "email"), "operator@example.invalid")
        self.assertEqual(migration.scalar(identity, "userID"), "original-subject")
        self.assertEqual(migration.scalar(identity, "hash"), "$2b$fixture")

    def test_identity_scalar_rejects_ambiguous_or_missing_fields(self):
        for value in ["", "    userID: first\n    userID: second\n", "    userID: [one, two]\n"]:
            with self.assertRaises(migration.MigrationError):
                migration.scalar(value, "userID")

    def test_snapshot_preserves_wal_queue_identity_and_sequence(self):
        source = self.root / "source.sqlite3"
        before = sqlite3.connect(source)
        self.addCleanup(before.close)
        before.execute("PRAGMA journal_mode=WAL")
        before.execute("CREATE TABLE queue(event_id TEXT, sequence INTEGER, body TEXT)")
        before.execute("INSERT INTO queue VALUES('original-event',123456789,'original-body')")
        before.commit()
        target = self.root / "copy/queue.sqlite3"
        migration.sqlite_snapshot(source, target)
        with contextlib.closing(sqlite3.connect(target)) as after:
            self.assertEqual(after.execute("SELECT * FROM queue").fetchall(),
                             [("original-event", 123456789, "original-body")])
        self.assertEqual(target.stat().st_mode & 0o777, 0o600)
        self.assertEqual(before.execute("SELECT count(*) FROM queue").fetchone()[0], 1)

    def test_snapshot_refuses_source_symlink(self):
        source = self.root / "db"
        source.write_text("")
        link = self.root / "link"
        link.symlink_to(source)
        with self.assertRaises(migration.MigrationError):
            migration.sqlite_snapshot(link, self.root / "target")

    def test_final_snapshot_refuses_loaded_local_writer_before_dump_or_copy(self):
        (self.root / "metadata.json").write_text("{}")
        (self.root / "source-inputs.json").write_text("{}")
        with mock.patch.object(migration, "source_inputs", return_value={}), \
             mock.patch.object(migration.subprocess, "run", return_value=mock.Mock(returncode=0)) as run:
            with self.assertRaises(migration.MigrationError):
                migration.export_snapshot(self.root, self.root, self.root, final=True)
        self.assertEqual(run.call_count, 1)
        self.assertEqual(run.call_args.args[0][:2], ["launchctl", "print"])
        self.assertFalse((self.root / "snapshot.json").exists())

    def test_configuration_drift_blocks_snapshot_before_local_or_destination_commands(self):
        (self.root / "metadata.json").write_text("{}")
        (self.root / "source-inputs.json").write_text('{"files":{"credential":"old"}}')
        with mock.patch.object(migration, "source_inputs", return_value={"files": {"credential": "new"}}), \
             mock.patch.object(migration.subprocess, "run") as run:
            with self.assertRaises(migration.MigrationError):
                migration.export_snapshot(self.root, self.root, self.root, final=True)
        run.assert_not_called()

    def test_bundle_manifest_is_repeatable_and_detects_extra_or_modified_files(self):
        migration.write(self.root / "private/config", "original")
        migration.seal(self.root)
        first = json.loads((self.root / "manifest.json").read_text())
        migration.seal(self.root)
        self.assertEqual(first, json.loads((self.root / "manifest.json").read_text()))
        migration.write(self.root / "private/config", "changed")
        self.assertNotEqual(first, migration.manifest(self.root))
        migration.write(self.root / "extra", "extra")
        self.assertEqual(len(migration.manifest(self.root)["files"]), 2)

    def test_bundle_manifest_refuses_secret_symlinks(self):
        (self.root / "escape").symlink_to("/etc/passwd")
        with self.assertRaises(migration.MigrationError):
            migration.manifest(self.root)

    def test_tree_refuses_symlink_and_preserves_executable(self):
        source = self.root / "source"
        source.mkdir()
        program = source / "program"
        migration.write(program, "#!/bin/sh\nexit 0\n", True)
        migration.tree(source, self.root / "copy")
        self.assertTrue(os.access(self.root / "copy/program", os.X_OK))
        (source / "escape").symlink_to("/etc/passwd")
        with self.assertRaises(migration.MigrationError):
            migration.tree(source, self.root / "copy2")

    def test_archive_refuses_traversal_absolute_and_links_before_extraction(self):
        for name, kind in [("../escaped", tarfile.REGTYPE), ("/escaped", tarfile.REGTYPE),
                           ("link", tarfile.SYMTYPE), ("hard", tarfile.LNKTYPE)]:
            archive = self.root / "archive.tar.gz"
            with tarfile.open(archive, "w:gz") as output:
                entry = tarfile.TarInfo(name)
                entry.type = kind
                entry.linkname = "/etc/passwd" if kind != tarfile.REGTYPE else ""
                entry.size = 1 if kind == tarfile.REGTYPE else 0
                output.addfile(entry, io.BytesIO(b"x") if kind == tarfile.REGTYPE else None)
            with self.subTest(name=name), self.assertRaises(migration.MigrationError):
                migration.unpack_release(archive, self.root / "extracted")
            self.assertFalse((self.root / "extracted").exists())

    def test_regular_release_extracts_without_world_readable_secrets(self):
        archive = self.root / "archive.tar.gz"
        with tarfile.open(archive, "w:gz") as output:
            entry = tarfile.TarInfo("manifest.json")
            entry.size = 2
            output.addfile(entry, io.BytesIO(b"{}"))
        migration.unpack_release(archive, self.root / "extracted")
        self.assertEqual((self.root / "extracted/manifest.json").read_text(), "{}")


if __name__ == "__main__":
    unittest.main()
