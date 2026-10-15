#!/usr/bin/env python3
"""Read Sentry's scanner and durably publish observations to Agent Room."""
from __future__ import annotations

import argparse
import contextlib
import dataclasses
import datetime as dt
import fcntl
import hashlib
import importlib
import ipaddress
import json
import os
from pathlib import Path
import signal
import sqlite3
import stat
import sys
import threading
import time
import tomllib
from typing import Callable
import urllib.error
import urllib.parse
import urllib.request
import uuid


class ConnectorError(Exception):
    """Only fixed, non-secret operational messages belong in this exception."""


@dataclasses.dataclass(frozen=True)
class ProjectMapping:
    path: Path
    project_id: str
    token_file: Path


@dataclasses.dataclass(frozen=True)
class Config:
    sentry_checkout: Path
    sentry_config: Path
    state_dir: Path
    instance_id: str
    ingest_url: str
    projects: tuple[ProjectMapping, ...]
    poll_seconds: float = 300
    timeout_seconds: float = 5
    batch_size: int = 50
    max_batch_bytes: int = 512 * 1024


def load_config(path: Path) -> Config:
    raw = tomllib.loads(path.read_text())
    settings = raw["connector"]
    checkout = Path(settings["sentry_checkout"]).expanduser().resolve()
    projects = tuple(ProjectMapping(
        Path(item["path"]).expanduser().resolve(), item["project_id"],
        Path(item["token_file"]).expanduser().absolute(),
    ) for item in raw.get("projects", []))
    config = Config(
        checkout, Path(settings.get("sentry_config", checkout / "sentry.toml")).expanduser().resolve(),
        Path(settings["state_dir"]).expanduser().absolute(), settings["instance_id"],
        settings["ingest_url"], projects, float(settings.get("poll_seconds", 300)),
        float(settings.get("timeout_seconds", 5)), int(settings.get("batch_size", 50)),
        int(settings.get("max_batch_bytes", 512 * 1024)),
    )
    endpoint = urllib.parse.urlsplit(config.ingest_url)
    host = endpoint.hostname or ""
    try:
        loopback = ipaddress.ip_address(host).is_loopback
    except ValueError:
        loopback = host == "localhost"
    if endpoint.scheme not in ("http", "https") or (endpoint.scheme == "http" and not loopback) or not host or endpoint.username or endpoint.password or endpoint.query or endpoint.fragment:
        raise ConnectorError("ingest_url must be HTTPS or loopback HTTP without credentials or query")
    if endpoint.path != "/api/v1/ingest":
        raise ConnectorError("ingest_url must name the private /api/v1/ingest endpoint")
    if not config.instance_id or len(config.instance_id) > 128 or not projects:
        raise ConnectorError("instance_id and explicit project mappings are required")
    if len({item.path for item in projects}) != len(projects) or len({item.project_id for item in projects}) != len(projects):
        raise ConnectorError("project paths and project IDs must each be unique")
    if any(not item.project_id or len(item.project_id) > 256 for item in projects):
        raise ConnectorError("project IDs must be nonempty and bounded")
    if not 1 <= config.batch_size <= 100 or not 16 * 1024 <= config.max_batch_bytes <= 1024 * 1024 or not 1 <= config.timeout_seconds <= 30 or config.poll_seconds < 10:
        raise ConnectorError("invalid polling, transport, or batch limits")
    return config


def timestamp(epoch: float) -> str:
    return dt.datetime.fromtimestamp(epoch, dt.timezone.utc).isoformat(timespec="microseconds").replace("+00:00", "Z")


def canonical(value: object) -> str:
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True)


def private_directory(path: Path) -> None:
    if path.is_symlink():
        raise ConnectorError("state directory must not be a symlink")
    path.mkdir(mode=0o700, parents=True, exist_ok=True)
    info = path.stat()
    if info.st_uid != os.getuid() or stat.S_IMODE(info.st_mode) & 0o077:
        raise ConnectorError("state directory must be owned by this user with mode 0700")


def private_file(path: Path, create: bool = False) -> int:
    flags = os.O_RDWR | os.O_CREAT if create else os.O_RDONLY
    fd = os.open(path, flags | os.O_NOFOLLOW, 0o600)
    info = os.fstat(fd)
    if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or stat.S_IMODE(info.st_mode) & 0o077:
        os.close(fd)
        raise ConnectorError("private file must be owned by this user with mode 0600")
    return fd


@contextlib.contextmanager
def process_lock(directory: Path):
    private_directory(directory)
    fd = private_file(directory / "connector.lock", create=True)
    try:
        try:
            fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise ConnectorError("another connector process owns this state directory") from None
        yield
    finally:
        os.close(fd)


class SentryReader:
    """Reuse Sentry's parsing and Git reads; never call board or history writers."""

    def __init__(self, config: Config):
        sys.path.insert(0, str(config.sentry_checkout))
        self.scanner = importlib.import_module("sentry.scan")
        config_module = importlib.import_module("sentry.config")
        self.rules = importlib.import_module("sentry.rules")
        for module in (self.scanner, config_module, self.rules):
            if not Path(module.__file__).resolve().is_relative_to(config.sentry_checkout):
                raise ConnectorError("Sentry modules must come from the configured checkout")
        self.config = config_module.load(config.sentry_config)

    def collect(self, now: float) -> dict:
        paths, skipped, unreadable = self.scanner.discover(self.config.roots, self.config.exclude)
        found: dict[str, dict] = {}
        for path in paths:
            key = str(path.resolve())
            try:
                project = self.scanner.scan(path, self.config.owners)
                if project.upstream:
                    continue
                stage, reason = self.rules.classify(project, now, self.config.thresholds)
                tasks = self.scanner.read_tasks(path, now, self.config.tasks_window_days) if project.has_memory_bank else self.scanner.TaskScan()
                kept = [(str(p.resolve()), source, ident) for p, source, ident in tasks.kept]
                found[key] = {
                    "name": project.name, "status": stage.value, "reason": reason,
                    "scan_status": "partial" if kept else "complete", "items": tasks.items,
                    "kept": kept,
                    "scan_error": "Some task records were retained without rereading (window or unreadable)" if kept else "",
                }
            except OSError:
                unreadable.append(path)
            except Exception as error:
                # A parser failure belongs to this project, never in another project's checkpoint.
                found[key] = {"name": path.name, "status": "Unknown", "reason": "Source read failed", "scan_status": "unreadable", "items": [], "kept": [], "scan_error": type(error).__name__}
        return {"projects": found, "unreadable": [str(path.resolve()) for path in unreadable], "skipped": [str(path.resolve()) for path in skipped]}


class State:
    def __init__(self, directory: Path):
        private_directory(directory)
        self.path = directory / "queue.sqlite3"
        os.close(private_file(self.path, create=True))
        self.db = sqlite3.connect(self.path)
        self.db.row_factory = sqlite3.Row
        self.db.execute("PRAGMA journal_mode=WAL")
        self.db.execute("PRAGMA synchronous=FULL")
        self.db.executescript("""
            CREATE TABLE IF NOT EXISTS metadata(key TEXT PRIMARY KEY,value TEXT NOT NULL);
            CREATE TABLE IF NOT EXISTS observations(project_id TEXT NOT NULL,external_id TEXT NOT NULL,fingerprint TEXT NOT NULL,sequence INTEGER NOT NULL,PRIMARY KEY(project_id,external_id));
            CREATE TABLE IF NOT EXISTS queue(id INTEGER PRIMARY KEY,project_id TEXT NOT NULL,event_id TEXT UNIQUE NOT NULL,body TEXT NOT NULL,occurred_at REAL NOT NULL,status TEXT NOT NULL DEFAULT 'pending',attempts INTEGER NOT NULL DEFAULT 0,next_attempt REAL NOT NULL DEFAULT 0,error TEXT);
            CREATE INDEX IF NOT EXISTS queue_delivery ON queue(project_id,status,id);
            CREATE TABLE IF NOT EXISTS projects(project_id TEXT PRIMARY KEY,report TEXT NOT NULL,last_success TEXT,last_delivery TEXT,delivery_error TEXT);
            CREATE TABLE IF NOT EXISTS project_bindings(project_id TEXT PRIMARY KEY,path TEXT UNIQUE NOT NULL);
        """)

    def close(self):
        self.db.close()

    def set_meta(self, key: str, value: object):
        self.db.execute("INSERT INTO metadata VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", (key, canonical(value)))

    def meta(self, key: str, default=None):
        row = self.db.execute("SELECT value FROM metadata WHERE key=?", (key,)).fetchone()
        return json.loads(row[0]) if row else default

    def enqueue(self, config: Config, mapping: ProjectMapping, event_type: str, payload: dict, now: float, deduplicate: bool):
        external_id = payload["external_id"]
        fingerprint = hashlib.sha256(canonical(payload).encode()).hexdigest()
        key = event_type + ":" + external_id
        old = self.db.execute("SELECT fingerprint FROM observations WHERE project_id=? AND external_id=?", (mapping.project_id, key)).fetchone()
        if deduplicate and old and old[0] == fingerprint:
            return
        sequence = max(int(self.meta("sequence", 0)) + 1, time.time_ns())
        if sequence >= 2**63:
            raise ConnectorError("source sequence exhausted int64 range")
        self.set_meta("sequence", sequence)
        event_id = config.instance_id + ":" + uuid.uuid4().hex
        event = {"project_id": mapping.project_id, "subject_id": mapping.project_id if event_type == "project.observed" else external_id,
                 "subject_type": "project" if event_type == "project.observed" else "task", "type": event_type,
                 "schema_version": 1, "source_system": "sentry", "source_event_id": event_id,
                 "source_sequence": sequence, "occurred_at": timestamp(now), "payload": payload}
        self.db.execute("INSERT INTO queue(project_id,event_id,body,occurred_at) VALUES(?,?,?,?)", (mapping.project_id, event_id, canonical(event), now))
        self.db.execute("INSERT INTO observations VALUES(?,?,?,?) ON CONFLICT(project_id,external_id) DO UPDATE SET fingerprint=excluded.fingerprint,sequence=excluded.sequence", (mapping.project_id, key, fingerprint, sequence))

    def status(self) -> dict:
        rows = self.db.execute("SELECT project_id,report,last_success,last_delivery,delivery_error FROM projects ORDER BY project_id").fetchall()
        counts = dict(self.db.execute("SELECT status,count(*) FROM queue GROUP BY status").fetchall())
        quarantined = [dict(row) for row in self.db.execute("SELECT project_id,event_id,error,occurred_at FROM queue WHERE status='quarantined' ORDER BY id")]
        return {"initialized": True, "last_collection": self.meta("last_collection"), "collection_error": self.meta("collection_error"),
                "needs_provisioning": self.meta("needs_provisioning", []), "pending": counts.get("pending", 0),
                "quarantined": counts.get("quarantined", 0), "quarantine": quarantined,
                "transport_error": self.meta("transport_error"),
                "unreadable": self.meta("unreadable", []),
                "projects": [{"project_id": row["project_id"], **json.loads(row["report"]), "last_success": row["last_success"], "last_delivery": row["last_delivery"], "delivery_error": row["delivery_error"]} for row in rows]}


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def publish(config: Config, mapping: ProjectMapping, body: bytes) -> str | None:
    try:
        with os.fdopen(private_file(mapping.token_file), "r") as token_file:
            token = token_file.read(16385).strip()
        if not token or len(token) > 16384 or any(ord(char) <= 32 or ord(char) >= 127 for char in token):
            return "invalid_token_file"
        target = config.ingest_url + "?" + urllib.parse.urlencode({"project_id": mapping.project_id})
        request = urllib.request.Request(target, body, {"Authorization": "Bearer " + token, "Content-Type": "application/json", "Accept": "application/json"}, method="POST")
        with urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect()).open(request, timeout=config.timeout_seconds) as response:
            return None if response.status == 202 else f"http_{response.status}"
    except urllib.error.HTTPError as error:
        status = error.code
        error.close()
        return f"http_{status}"
    except (urllib.error.URLError, TimeoutError, ConnectionError):
        return "transport_unavailable"
    except (OSError, ConnectorError, UnicodeError):
        return "private_token_unavailable"


class Collector:
    def __init__(self, config: Config, state: State, reader, sender: Callable | None = None):
        self.config, self.state, self.reader, self.sender = config, state, reader, sender or publish
        with state.db:
            instance = state.meta("instance_id")
            if instance is not None and instance != config.instance_id:
                raise ConnectorError("state directory belongs to another source instance")
            state.set_meta("instance_id", config.instance_id)
            for mapping in config.projects:
                bindings = state.db.execute("SELECT project_id,path FROM project_bindings WHERE project_id=? OR path=?", (mapping.project_id, str(mapping.path))).fetchall()
                if any(row["project_id"] != mapping.project_id or row["path"] != str(mapping.path) for row in bindings):
                    raise ConnectorError("project identity binding changed; an explicit state migration is required")
                state.db.execute("INSERT OR IGNORE INTO project_bindings VALUES(?,?)", (mapping.project_id, str(mapping.path)))

    def collect(self, now: float):
        try:
            scan = self.reader.collect(now)
        except Exception as error:
            with self.state.db:
                self.state.set_meta("collection_error", type(error).__name__)
            return
        known = {str(mapping.path) for mapping in self.config.projects}
        unknown = [{"path": path, "name": report["name"]} for path, report in sorted(scan["projects"].items()) if path not in known]
        with self.state.db:
            self.state.set_meta("needs_provisioning", unknown)
            self.state.set_meta("unreadable", scan["unreadable"])
            self.state.set_meta("last_collection", timestamp(now))
            self.state.set_meta("collection_error", None)
            for mapping in self.config.projects:
                report = scan["projects"].get(str(mapping.path))
                if report is None:
                    report = {"name": mapping.path.name, "status": "Unknown", "reason": "Configured project was not readable in this scan", "scan_status": "unreadable", "items": [], "kept": [], "scan_error": "Project is unreadable or no longer discovered; previous records retained"}
                payload = {"external_id": self.config.instance_id + ":" + str(mapping.path), "name": report["name"], "path": str(mapping.path), "status": report["status"], "reason": report["reason"],
                           "collected_at": timestamp(now), "scan_status": report["scan_status"], "task_count": len(report["items"])}
                if report["scan_error"]:
                    payload["scan_error"] = report["scan_error"]
                self.state.enqueue(self.config, mapping, "project.observed", payload, now, False)
                for item in report["items"]:
                    description = "\n".join(([f"Source excerpt: {item.source}"] + ([f"Section: {item.heading}"] if item.heading else []) + [text for _, text in item.detail]))
                    if len(description) > 16384:
                        suffix = "\n[Source excerpt truncated; inspect the source file for the complete record.]"
                        description = description[:16384 - len(suffix)] + suffix
                    task = {"external_id": item.source + "#" + item.ident, "title": item.title, "status": item.status.value, "description": description}
                    self.state.enqueue(self.config, mapping, "task.observed", task, now, True)
                local_report = {"collected_at": timestamp(now), "scan_status": report["scan_status"], "task_count": len(report["items"]), "kept": report["kept"], "scan_error": report["scan_error"] or None}
                self.state.db.execute("INSERT INTO projects(project_id,report,last_success) VALUES(?,?,?) ON CONFLICT(project_id) DO UPDATE SET report=excluded.report,last_success=COALESCE(excluded.last_success,projects.last_success)",
                                      (mapping.project_id, canonical(local_report), timestamp(now) if report["scan_status"] == "complete" else None))

    def deliver(self, now: float):
        if self.state.meta("transport_retry", 0) > now:
            return
        with self.state.db:
            self.state.db.execute("UPDATE queue SET status='quarantined',error='observation_older_than_30_days_requires_review' WHERE status='pending' AND occurred_at<?", (now - 30 * 86400,))
        for mapping in self.config.projects:
            rows = self.state.db.execute("SELECT * FROM queue WHERE project_id=? AND status='pending' ORDER BY id LIMIT ?", (mapping.project_id, self.config.batch_size)).fetchall()
            if not rows or rows[0]["next_attempt"] > now:
                continue
            batch = []
            size = len(b'{"events":[]}')
            for row in rows:
                event_bytes = row["body"].encode()
                if len(event_bytes) + size + 1 > self.config.max_batch_bytes:
                    break
                batch.append(row)
                size += len(event_bytes) + 1
            if not batch:
                with self.state.db:
                    self.state.db.execute("UPDATE queue SET status='quarantined',error='event_exceeds_batch_limit_requires_review' WHERE id=?", (rows[0]["id"],))
                continue
            body = b'{"events":[' + b','.join(row["body"].encode() for row in batch) + b']}'
            try:
                error = self.sender(self.config, mapping, body)
            except Exception:
                error = "transport_unavailable"
            with self.state.db:
                if error is None:
                    self.state.db.executemany("DELETE FROM queue WHERE id=?", [(row["id"],) for row in batch])
                    self.state.db.execute("UPDATE projects SET last_delivery=?,delivery_error=NULL WHERE project_id=?", (timestamp(now), mapping.project_id))
                    self.state.set_meta("transport_error", None)
                    self.state.set_meta("transport_failures", 0)
                else:
                    attempts = batch[0]["attempts"] + 1
                    delay = min(900, 5 * 2**min(attempts - 1, 8))
                    permanent = error in ("http_400", "http_409", "http_413", "http_422")
                    self.state.db.executemany("UPDATE queue SET attempts=attempts+1,next_attempt=?,error=?,status=? WHERE id=?", [(now + delay, error, "quarantined" if permanent else "pending", row["id"]) for row in batch])
                    self.state.db.execute("UPDATE projects SET delivery_error=? WHERE project_id=?", (error, mapping.project_id))
                    if error in ("transport_unavailable", "http_502", "http_503", "http_504"):
                        failures = self.state.meta("transport_failures", 0) + 1
                        self.state.set_meta("transport_failures", failures)
                        self.state.set_meta("transport_retry", now + min(900, 5 * 2**min(failures - 1, 8)))
                        self.state.set_meta("transport_error", error)
            if error in ("transport_unavailable", "http_502", "http_503", "http_504"):
                break


def read_status(config: Config) -> dict:
    path = config.state_dir / "queue.sqlite3"
    if not path.exists():
        return {"initialized": False, "pending": 0, "quarantined": 0}
    os.close(private_file(path))
    state = State.__new__(State)
    state.db = sqlite3.connect(path.as_uri() + "?mode=ro", uri=True)
    state.db.row_factory = sqlite3.Row
    try:
        return state.status()
    finally:
        state.close()


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, epilog="once collects one pass and sends one bounded batch per project; exit 2 means durable work remains pending, exit 1 means an error or quarantine requires attention.")
    parser.add_argument("--config", type=Path, required=True)
    parser.add_argument("command", choices=("once", "watch", "status"))
    args = parser.parse_args()
    os.umask(0o077)
    try:
        config = load_config(args.config)
        if args.command == "status":
            print(canonical(read_status(config)))
            return 0
        with process_lock(config.state_dir):
            state = State(config.state_dir)
            try:
                collector = Collector(config, state, SentryReader(config))
                stopped = threading.Event()
                for signum in (signal.SIGTERM, signal.SIGINT):
                    signal.signal(signum, lambda *_: stopped.set())
                next_collection = 0.0
                while not stopped.is_set():
                    if time.monotonic() >= next_collection:
                        collector.collect(time.time())
                        next_collection = time.monotonic() + config.poll_seconds
                    collector.deliver(time.time())
                    status = state.status()
                    if args.command == "once":
                        print(canonical(status))
                        if status["collection_error"] or status["transport_error"] or status["quarantined"] or any(item["delivery_error"] for item in status["projects"]):
                            return 1
                        return 2 if status["pending"] else 0
                    stopped.wait(min(5, max(0.1, next_collection - time.monotonic())))
            finally:
                state.close()
        return 0
    except Exception as error:
        message = str(error) if isinstance(error, ConnectorError) else type(error).__name__
        print(canonical({"error": message}), file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
