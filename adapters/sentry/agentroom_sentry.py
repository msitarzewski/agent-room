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


# Both executable providers import the same delivery module from any working directory.
sys.path.insert(0, str(Path(__file__).resolve().parents[2]))
from adapters.source import (ConnectorError, Delivery, NoRedirect, State, canonical,
                             private_directory, private_file, process_lock, publish,
                             read_status, timestamp, validate_ingest_url)


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
    validate_ingest_url(config.ingest_url)
    if not config.instance_id or len(config.instance_id) > 128 or not projects:
        raise ConnectorError("instance_id and explicit project mappings are required")
    if len({item.path for item in projects}) != len(projects) or len({item.project_id for item in projects}) != len(projects):
        raise ConnectorError("project paths and project IDs must each be unique")
    if any(not item.project_id or len(item.project_id) > 256 for item in projects):
        raise ConnectorError("project IDs must be nonempty and bounded")
    if not 1 <= config.batch_size <= 100 or not 16 * 1024 <= config.max_batch_bytes <= 1024 * 1024 or not 1 <= config.timeout_seconds <= 30 or config.poll_seconds < 10:
        raise ConnectorError("invalid polling, transport, or batch limits")
    return config


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


class Collector(Delivery):
    def __init__(self, config: Config, state: State, reader, sender: Callable | None = None):
        self.config, self.state, self.reader, self.sender = config, state, reader, sender or publish
        with state.db:
            state.bind_source(config.instance_id, "sentry")
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
