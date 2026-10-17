"""Shared private source queue and immutable Agent Room delivery; standard library only."""
from __future__ import annotations

import contextlib
import datetime as dt
import fcntl
import hashlib
import ipaddress
import json
import os
from pathlib import Path
import sqlite3
import stat
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid

class ConnectorError(Exception):
    """Only fixed, non-secret operational messages belong in this exception."""


def validate_ingest_url(value: str):
    endpoint = urllib.parse.urlsplit(value)
    host = endpoint.hostname or ""
    try:
        loopback = ipaddress.ip_address(host).is_loopback
    except ValueError:
        loopback = host == "localhost"
    if endpoint.scheme not in ("http", "https") or (endpoint.scheme == "http" and not loopback) or not host or endpoint.username or endpoint.password or endpoint.query or endpoint.fragment:
        raise ConnectorError("ingest_url must be HTTPS or loopback HTTP without credentials or query")
    if endpoint.path != "/api/v1/ingest":
        raise ConnectorError("ingest_url must name the private /api/v1/ingest endpoint")




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

    def bind_source(self, instance_id: str, source_system: str):
        if self.meta("instance_id", instance_id) != instance_id or self.meta("source_system", source_system) != source_system:
            raise ConnectorError("state directory belongs to another source instance or provider")
        if source_system != "sentry" and self.meta("source_system") is None and self.db.execute("SELECT 1 FROM project_bindings LIMIT 1").fetchone():
            raise ConnectorError("legacy Sentry state cannot be reused for another provider")
        self.set_meta("instance_id", instance_id)
        self.set_meta("source_system", source_system)

    def set_meta(self, key: str, value: object):
        self.db.execute("INSERT INTO metadata VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", (key, canonical(value)))

    def meta(self, key: str, default=None):
        row = self.db.execute("SELECT value FROM metadata WHERE key=?", (key,)).fetchone()
        return json.loads(row[0]) if row else default

    def enqueue(self, config, mapping, event_type: str, payload: dict, now: float, deduplicate: bool):
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
                 "schema_version": 1, "source_system": getattr(config, "source_system", "sentry"), "source_event_id": event_id,
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
                "provider_status": self.meta("provider_status"),
                "unreadable": self.meta("unreadable", []),
                "projects": [{"project_id": row["project_id"], **json.loads(row["report"]), "last_success": row["last_success"], "last_delivery": row["last_delivery"], "delivery_error": row["delivery_error"]} for row in rows]}


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def publish(config, mapping, body: bytes) -> str | None:
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




class Delivery:
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



def read_status(config) -> dict:
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


