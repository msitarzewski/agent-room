#!/usr/bin/env python3
"""Observe GitHub issues, pull requests, reviews and current-head CI; never write GitHub."""
from __future__ import annotations

import argparse
import dataclasses
import datetime as dt
import email.utils
import json
import os
from pathlib import Path
import re
import signal
import sys
import threading
import time
import tomllib
import urllib.error
import urllib.parse
import urllib.request

sys.path.insert(0, str(Path(__file__).resolve().parents[2]))
from adapters.source import (ConnectorError, Delivery, NoRedirect, State, canonical,
                             private_file, process_lock, publish, read_status, timestamp)

API_VERSION = "2026-03-10"
API_ORIGIN = "https://api.github.com"
BODY_LIMIT = 16384
NAME = re.compile(r"[A-Za-z0-9][A-Za-z0-9_.-]{0,99}\Z")
SHA = re.compile(r"[a-fA-F0-9]{40,64}\Z")


@dataclasses.dataclass(frozen=True)
class Repository:
    owner: str
    name: str
    project_id: str
    token_file: Path
    repository_id: int | None = None

    @property
    def path(self):
        return f"/repos/{self.owner}/{self.name}"


@dataclasses.dataclass(frozen=True)
class Config:
    state_dir: Path
    instance_id: str
    ingest_url: str
    projects: tuple[Repository, ...]
    poll_seconds: float = 900
    timeout_seconds: float = 5
    batch_size: int = 50
    max_batch_bytes: int = 512 * 1024
    request_budget: int = 12
    max_pages: int = 10
    github_token_file: Path | None = None
    source_system: str = dataclasses.field(default="github", init=False)


def load_config(path: Path) -> Config:
    raw = tomllib.loads(path.read_text())
    settings = raw["connector"]
    repositories = tuple(Repository(item["owner"], item["name"], item["project_id"],
                                   Path(item["token_file"]).expanduser().absolute(), item.get("repository_id"))
                         for item in raw.get("repositories", []))
    config = Config(Path(settings["state_dir"]).expanduser().absolute(), settings["instance_id"],
                    settings["ingest_url"], repositories,
                    float(settings.get("poll_seconds", 900)), float(settings.get("timeout_seconds", 5)),
                    int(settings.get("batch_size", 50)), int(settings.get("max_batch_bytes", 512 * 1024)),
                    int(settings.get("request_budget", 12)), int(settings.get("max_pages", 10)),
                    Path(settings["github_token_file"]).expanduser().absolute() if settings.get("github_token_file") else None)
    from adapters.source import validate_ingest_url
    validate_ingest_url(config.ingest_url)
    if not config.instance_id or len(config.instance_id) > 128 or not repositories:
        raise ConnectorError("instance_id and explicit repository mappings are required")
    if len({r.project_id for r in repositories}) != len(repositories) or len({(r.owner.lower(), r.name.lower()) for r in repositories}) != len(repositories):
        raise ConnectorError("repository and project mappings must each be unique")
    for repo in repositories:
        if not NAME.fullmatch(repo.owner) or not NAME.fullmatch(repo.name) or not repo.project_id or len(repo.project_id) > 256:
            raise ConnectorError("invalid bounded repository or project identity")
        if repo.repository_id is not None and (type(repo.repository_id) is not int or repo.repository_id <= 0):
            raise ConnectorError("repository_id must be a positive native numeric ID")
    if not 1 <= config.batch_size <= 100 or not 16384 <= config.max_batch_bytes <= 1048576 or not 1 <= config.timeout_seconds <= 30 or not 60 <= config.poll_seconds <= 86400 or not 4 <= config.request_budget <= 100 or not 1 <= config.max_pages <= 100:
        raise ConnectorError("invalid polling, transport, pagination or request budget")
    return config


class FetchError(ConnectorError):
    pass


def bounded(value: str, limit: int = BODY_LIMIT) -> str:
    if len(value) <= limit:
        return value
    suffix = "\n[GitHub source body truncated; open the source URL for the complete record.]"
    return value[:limit - len(suffix)] + suffix


def parse_time(value: str) -> float:
    try:
        return dt.datetime.fromisoformat(value.replace("Z", "+00:00")).timestamp()
    except (ValueError, AttributeError):
        raise FetchError("github_invalid_timestamp") from None


class GitHub:
    """Serial, bounded GETs only. Cache writes share the observation transaction."""
    def __init__(self, config: Config, state: State, opener=None):
        self.config, self.state = config, state
        self.opener = opener or urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())
        self.remaining = config.request_budget
        self.repo: Repository | None = None
        self.now = 0.0

    def begin(self, now: float):
        self.now, self.remaining = now, self.config.request_budget

    def allowed(self, path: str):
        parsed = urllib.parse.urlsplit(path)
        if parsed.scheme or parsed.netloc or parsed.fragment or not self.repo:
            raise FetchError("github_unsafe_api_endpoint")
        suffix = parsed.path.removeprefix(self.repo.path)
        if not parsed.path.startswith(self.repo.path) or not re.fullmatch(r"(?:|/issues|/pulls/[1-9][0-9]*(?:/reviews)?|/commits/[a-fA-F0-9]{40,64}/(?:check-runs|status))", suffix):
            raise FetchError("github_unsafe_api_endpoint")
        if any(key not in {"state", "sort", "direction", "since", "per_page", "page", "filter"} for key in urllib.parse.parse_qs(parsed.query)):
            raise FetchError("github_unsafe_api_query")

    def _token(self):
        if self.config.github_token_file is None:
            return None
        try:
            with os.fdopen(private_file(self.config.github_token_file)) as handle:
                token = handle.read(16385).strip()
            if not token or len(token) > 16384 or any(ord(char) <= 32 or ord(char) >= 127 for char in token):
                raise FetchError("github_invalid_token_file")
            return token
        except (OSError, ConnectorError, UnicodeError):
            raise FetchError("github_private_token_unavailable") from None

    def defer(self, headers, code: int):
        until = self.now + 60
        retry = headers.get("Retry-After")
        try:
            if retry:
                try:
                    until = self.now + max(1, int(retry))
                except ValueError:
                    until = email.utils.parsedate_to_datetime(retry).timestamp()
            elif headers.get("X-RateLimit-Remaining") == "0":
                until = float(headers.get("X-RateLimit-Reset", until)) + 1
            elif code in (403, 429):
                failures = self.state.meta("github_rate_failures", 0) + 1
                self.state.set_meta("github_rate_failures", failures)
                until = self.now + min(3600, 60 * 2 ** min(failures - 1, 6))
        except (ValueError, TypeError, OverflowError):
            until = self.now + 60
        self.state.set_meta("github_next_request", max(self.now + 1, until))

    def get(self, path: str):
        self.allowed(path)
        if self.state.meta("github_next_request", 0) > self.now:
            raise FetchError("github_rate_limit_backoff")
        if self.remaining <= 0:
            raise FetchError("github_request_budget_exhausted")
        self.remaining -= 1
        cached = self.state.db.execute("SELECT * FROM github_cache WHERE path=?", (path,)).fetchone()
        headers = {"Accept": "application/vnd.github+json", "X-GitHub-Api-Version": API_VERSION,
                   "User-Agent": "AgentRoom-readonly-monitor"}
        token = self._token()
        if token:
            headers["Authorization"] = "Bearer " + token
        if cached and cached["etag"]:
            headers["If-None-Match"] = cached["etag"]
        request = urllib.request.Request(API_ORIGIN + path, headers=headers, method="GET")
        try:
            try:
                response = self.opener.open(request, timeout=self.config.timeout_seconds)
            except urllib.error.HTTPError as error:
                response = error
            with response:
                code = response.code if isinstance(response, urllib.error.HTTPError) else response.status
                if response.headers.get("X-RateLimit-Remaining") == "0" or code in (403, 429):
                    self.defer(response.headers, code)
                if code == 304 and cached:
                    self.state.db.execute("UPDATE github_cache SET used_at=? WHERE path=?", (self.now, path))
                    return json.loads(cached["body"]), cached["link"]
                if code != 200:
                    raise FetchError(f"github_http_{code}")
                data = response.read(8 * 1024 * 1024 + 1)
                if len(data) > 8 * 1024 * 1024:
                    raise FetchError("github_response_limit_exceeded")
                try:
                    body = json.loads(data)
                except (ValueError, UnicodeError):
                    raise FetchError("github_invalid_json") from None
                etag, link = response.headers.get("ETag", ""), response.headers.get("Link", "")
                if len(etag) > 512 or len(link) > 8192 or any(c in etag for c in "\r\n"):
                    raise FetchError("github_invalid_cache_headers")
                self.state.db.execute("INSERT INTO github_cache VALUES(?,?,?,?,?) ON CONFLICT(path) DO UPDATE SET etag=excluded.etag,body=excluded.body,link=excluded.link,used_at=excluded.used_at", (path, etag, canonical(body), link, self.now))
                self.state.db.execute("DELETE FROM github_cache WHERE path IN (SELECT path FROM github_cache ORDER BY used_at DESC,path LIMIT -1 OFFSET 200)")
                return body, link
        except (urllib.error.URLError, TimeoutError, OSError):
            self.state.set_meta("github_next_request", self.now + 60)
            raise FetchError("github_transport_unavailable") from None

    def next_page(self, path: str, link: str):
        matches = re.findall(r'<([^>]+)>;\s*rel="next"', link)
        if not matches:
            return None
        if len(matches) != 1:
            raise FetchError("github_invalid_pagination")
        target = urllib.parse.urlsplit(matches[0])
        original = urllib.parse.urlsplit(path)
        query = urllib.parse.parse_qs(target.query)
        expected = urllib.parse.parse_qs(original.query)
        page = query.pop("page", ["1"])
        current_page = expected.pop("page", ["1"])
        if target.scheme != "https" or target.netloc != "api.github.com" or target.fragment or target.path != original.path or query != expected or len(page) != 1 or not page[0].isdigit() or int(page[0]) <= int(current_page[0]):
            raise FetchError("github_unsafe_pagination")
        result = target.path + "?" + target.query
        self.allowed(result)
        return result

    def listing(self, path: str, key: str | None = None, scope: str = ""):
        checkpoint = "github_list:" + scope + ":" + path
        progress = self.state.meta(checkpoint) or {}
        if self.now - progress.get("started_at", self.now) > 3600:
            progress = {}
            self.state.set_meta(checkpoint, None)
        records, seen = progress.get("records", []), set()
        path = progress.get("next", path)
        for _ in range(self.config.max_pages):
            if path in seen:
                raise FetchError("github_pagination_loop")
            seen.add(path)
            body, link = self.get(path)
            if key == "statuses" and (not isinstance(body, dict) or body.get("sha") != scope):
                raise FetchError("github_status_head_mismatch")
            values = body.get(key) if key and isinstance(body, dict) else body
            if not isinstance(values, list) or len(values) > 100:
                raise FetchError("github_invalid_list")
            records.extend(values)
            if len(records) > 10000 or len(canonical(records)) > 16 * 1024 * 1024:
                raise FetchError("github_list_record_limit_exceeded")
            following = self.next_page(path, link)
            if following is None:
                if key and body.get("total_count", len(records)) > len(records):
                    raise FetchError("github_incomplete_list")
                self.state.set_meta(checkpoint, None)
                return records
            path = following
            self.state.set_meta(checkpoint, {"next": path, "records": records, "started_at": progress.get("started_at", self.now)})
        raise FetchError("github_page_limit_exceeded")


class Collector(Delivery):
    def __init__(self, config: Config, state: State, api=None, sender=None):
        self.config, self.state, self.sender = config, state, sender or publish
        with state.db:
            state.bind_source(config.instance_id, "github")
        state.db.executescript("""
            CREATE TABLE IF NOT EXISTS github_bindings(project_id TEXT PRIMARY KEY,repository_id INTEGER UNIQUE NOT NULL);
            CREATE TABLE IF NOT EXISTS github_cache(path TEXT PRIMARY KEY,etag TEXT NOT NULL,body TEXT NOT NULL,link TEXT NOT NULL,used_at REAL NOT NULL);
            CREATE TABLE IF NOT EXISTS github_items(project_id TEXT NOT NULL,number INTEGER NOT NULL,kind TEXT NOT NULL,issue TEXT NOT NULL,detail TEXT,last_checked REAL NOT NULL DEFAULT 0,dirty INTEGER NOT NULL DEFAULT 1,PRIMARY KEY(project_id,number));
        """)
        self.api = api or GitHub(config, state)
        with state.db:
            for mapping in config.projects:
                prior = self.binding(mapping)
                if prior and mapping.repository_id and prior != mapping.repository_id:
                    raise ConnectorError("repository identity binding changed; explicit migration required")

    def binding(self, mapping: Repository):
        row = self.state.db.execute("SELECT repository_id FROM github_bindings WHERE project_id=?", (mapping.project_id,)).fetchone()
        return row[0] if row else None

    def bind(self, mapping: Repository, native_id):
        if type(native_id) is not int or native_id <= 0 or (mapping.repository_id and native_id != mapping.repository_id):
            raise FetchError("github_repository_identity_mismatch")
        prior = self.binding(mapping)
        collision = self.state.db.execute("SELECT project_id FROM github_bindings WHERE repository_id=?", (native_id,)).fetchone()
        if (prior and prior != native_id) or (collision and collision[0] != mapping.project_id):
            raise FetchError("github_repository_binding_requires_migration")
        self.state.db.execute("INSERT OR IGNORE INTO github_bindings VALUES(?,?)", (mapping.project_id, native_id))
        return native_id

    def save_issue(self, mapping, issue):
        if not isinstance(issue, dict) or type(issue.get("number")) is not int or issue["number"] <= 0 or not isinstance(issue.get("title"), str) or issue.get("state") not in ("open", "closed") or not isinstance(issue.get("body"), (str, type(None))):
            raise FetchError("github_invalid_issue")
        parse_time(issue.get("updated_at"))
        kind = "pull" if "pull_request" in issue else "issue"
        previous = self.state.db.execute("SELECT issue,kind FROM github_items WHERE project_id=? AND number=?", (mapping.project_id, issue["number"])).fetchone()
        if previous and previous["kind"] != kind:
            raise FetchError("github_native_kind_changed")
        encoded = canonical(issue)
        dirty = not previous or encoded != previous["issue"]
        self.state.db.execute("INSERT INTO github_items(project_id,number,kind,issue,dirty) VALUES(?,?,?,?,?) ON CONFLICT(project_id,number) DO UPDATE SET issue=excluded.issue,dirty=max(github_items.dirty,excluded.dirty)", (mapping.project_id, issue["number"], kind, encoded, int(dirty)))

    def issue_pages(self, mapping, now):
        key = f"github_scan:{mapping.project_id}"
        progress = self.state.meta(key, {})
        full_at = self.state.meta("github_full:" + mapping.project_id, 0)
        # Offset-based GitHub pagination is not a snapshot. Reconcile periodically
        # so concurrent issue movement cannot make a delta omission permanent.
        if not progress.get("next") and full_at and now - full_at >= 86400:
            progress = {}
        first = mapping.path + "/issues?state=all&sort=updated&direction=asc&per_page=20"
        if progress.get("since"):
            first += "&" + urllib.parse.urlencode({"since": progress["since"]})
        path, seen = progress.get("next", first), set()
        start = progress.get("started", now)
        for _ in range(min(self.config.max_pages, max(1, self.config.request_budget // 3))):
            if path in seen:
                raise FetchError("github_pagination_loop")
            seen.add(path)
            rows, link = self.api.get(path)
            if not isinstance(rows, list) or len(rows) > 100:
                raise FetchError("github_invalid_issue_list")
            for issue in rows:
                self.save_issue(mapping, issue)
            following = self.api.next_page(path, link)
            if following is None:
                self.state.set_meta(key, {"since": timestamp(start - 120), "completed_at": timestamp(now)})
                if not progress.get("since"):
                    self.state.set_meta("github_full:" + mapping.project_id, now)
                return "delta" if progress.get("since") else "full", True
            path = following
            progress.update({"next": path, "started": start})
            self.state.set_meta(key, progress)
        return "full-pagination" if not progress.get("since") else "delta-pagination", False

    def pull(self, mapping, row):
        detail, _ = self.api.get(mapping.path + f"/pulls/{row['number']}")
        if not isinstance(detail, dict) or detail.get("number") != row["number"] or detail.get("state") not in ("open", "closed") or type(detail.get("merged")) is not bool or type(detail.get("draft")) is not bool:
            raise FetchError("github_invalid_pull_detail")
        # Closed records need merge truth, not repeated expensive historical review/CI fetches.
        if detail["state"] == "closed":
            return detail, [], [], [], "closed; historical review/CI not refreshed", False
        head = detail.get("head", {}).get("sha", "")
        if not isinstance(head, str) or not SHA.fullmatch(head):
            raise FetchError("github_invalid_head_sha")
        cycle_key = f"github_pull_cycle:{mapping.project_id}:{row['number']}"
        cycle = self.state.meta(cycle_key) or {}
        if cycle.get("head") != head or self.api.now - parse_time(cycle.get("started_at", timestamp(self.api.now))) > 3600:
            cycle = {"head": head, "started_at": timestamp(self.api.now)}
        for name, path, key in (
            ("reviews", mapping.path + f"/pulls/{row['number']}/reviews?per_page=100", None),
            ("checks", mapping.path + f"/commits/{head}/check-runs?filter=latest&per_page=100", "check_runs"),
            ("statuses", mapping.path + f"/commits/{head}/status?per_page=100", "statuses"),
        ):
            if name not in cycle:
                cycle[name] = self.api.listing(path, key, head)
                self.state.set_meta(cycle_key, cycle)
        reviews, checks, statuses = cycle["reviews"], cycle["checks"], cycle["statuses"]
        if any(check.get("head_sha") != head for check in checks):
            raise FetchError("github_check_head_mismatch")
        self.state.set_meta(cycle_key, None)
        latest_checks, latest_statuses = {}, {}
        for check in sorted(checks, key=lambda c: c.get("id", 0)):
            latest_checks[(check.get("name"), check.get("app", {}).get("id"))] = check
        for status in sorted(statuses, key=lambda c: c.get("id", 0)):
            latest_statuses[status.get("context")] = status
        stale_cycle = self.api.now - parse_time(cycle["started_at"]) >= 900
        scope = "current head CI; branch requirements unknown"
        if stale_cycle:
            scope += "; partial cycle started " + cycle["started_at"]
        return detail, reviews, list(latest_checks.values()), list(latest_statuses.values()), scope, stale_cycle

    def task(self, mapping, native_id, row, now):
        issue = json.loads(row["issue"])
        number, kind = row["number"], row["kind"]
        status = issue["state"]
        parts = [f"GitHub {kind} #{number}", f"Source state: {issue['state']}", "GitHub updated_at: " + issue["updated_at"]]
        error = None
        if kind == "pull":
            try:
                detail, reviews, checks, statuses, scope, stale_cycle = self.pull(mapping, row)
                if stale_cycle:
                    error = "github_detail_cycle_spans_freshness_window"
                    parts.append("PARTIAL: carried review/CI facts are not a fresh complete collection")
                self.state.db.execute("UPDATE github_items SET detail=?,last_checked=?,dirty=0 WHERE project_id=? AND number=?", (canonical(detail), now, mapping.project_id, number))
                latest = {}
                for review in sorted(reviews, key=lambda r: (r.get("submitted_at") or "", r.get("id", 0))):
                    reviewer = review.get("user", {}).get("login")
                    if reviewer and review.get("state") in ("APPROVED", "CHANGES_REQUESTED", "DISMISSED"):
                        latest[reviewer] = review
                failing = any(check.get("conclusion") in ("failure", "timed_out", "cancelled", "action_required", "startup_failure", "stale") for check in checks) or any(item.get("state") in ("failure", "error") for item in statuses)
                changes = any(review["state"] == "CHANGES_REQUESTED" for review in latest.values())
                status = "merged" if detail["merged"] else "closed" if detail["state"] == "closed" else "blocked" if failing or changes else "draft" if detail["draft"] else "review"
                head = detail.get("head", {}).get("sha", "unknown")
                parts += [f"PR state: {status}; draft={detail['draft']}; merged={detail['merged']}", f"CI scope: {scope}", f"Head SHA: {head}"]
                if failing:
                    parts.append("Source blocker: observed current-head CI failure; required checks are unknown")
                if changes:
                    parts.append("Source blocker: latest effective reviewer state requests changes; review commits are listed below")
                review_bodies = []
                for reviewer, review in sorted(latest.items()):
                    parts.append(f"Review by {reviewer}: {review['state']} (commit {review.get('commit_id', 'unknown')}; current-head approval not inferred)")
                    if review.get("body"):
                        review_bodies.append(f"Review body by {reviewer}:\n{review['body']}")
                if not reviews:
                    parts.append("Reviews: none observed; approval is unknown")
                for check in checks:
                    parts.append(f"Check {check.get('name', 'unknown')}: {check.get('status', 'unknown')}/{check.get('conclusion') or 'pending'}")
                for item in statuses:
                    parts.append(f"Commit status {item.get('context', 'unknown')}: {item.get('state', 'unknown')}")
                if not checks and not statuses:
                    parts.append("CI: no current-head results observed; success is unknown")
                parts.extend(review_bodies)
                self.state.set_meta(f"github_known:{mapping.project_id}:{number}", {"status": status, "facts": bounded("\n\n".join(parts)), "at": timestamp(now)})
            except FetchError as failure:
                error = str(failure)
                known = self.state.meta(f"github_known:{mapping.project_id}:{number}")
                status = known["status"] if known else "closed" if issue["state"] == "closed" else "draft" if issue.get("draft") is True else "open"
                parts.append("PR review/CI/merge detail unavailable; not inferred from issue state")
                parts.append("Collection: " + error)
                if known:
                    parts.append("Prior detail is STALE, last successfully observed " + known["at"] + "; retained source status=" + status + "\n" + known["facts"])
            self.state.db.execute("UPDATE github_items SET last_checked=? WHERE project_id=? AND number=?", (now, mapping.project_id, number))
        else:
            self.state.db.execute("UPDATE github_items SET dirty=0,last_checked=? WHERE project_id=? AND number=?", (now, mapping.project_id, number))
        parts.append("Source body:\n" + (issue.get("body") or ""))
        payload = {"external_id": f"repo:{native_id}:{kind}:{number}", "title": bounded(f"{kind.upper()} #{number}: {issue['title']}", 4096), "status": status,
                   "description": bounded("\n\n".join(parts)), "external_url": f"https://github.com/{mapping.owner}/{mapping.name}/{'pull' if kind == 'pull' else 'issues'}/{number}"}
        self.state.enqueue(self.config, mapping, "task.observed", payload, now, True)
        return error

    def collect_repository(self, mapping, now):
        self.api.repo = mapping
        errors, observed, coverage = [], 0, "unreadable"
        native_id = self.binding(mapping) or mapping.repository_id
        repo = None
        try:
            repo, _ = self.api.get(mapping.path)
            if not isinstance(repo, dict) or str(repo.get("full_name", "")).lower() != f"{mapping.owner}/{mapping.name}".lower():
                raise FetchError("github_repository_name_mismatch")
            native_id = self.bind(mapping, repo.get("id"))
            previous = self.state.meta("github_repo:" + mapping.project_id)
            if previous and previous["path"] != mapping.path:
                self.state.set_meta(f"github_scan:{mapping.project_id}", {})
            self.state.set_meta("github_repo:" + mapping.project_id, {"path": mapping.path, "name": f"{mapping.owner}/{mapping.name}", "id": native_id})
        except FetchError as failure:
            # Never process old cached records under an unverified new repository URL.
            self.failed_repository(mapping, now, str(failure))
            return True
        try:
            coverage, finished = self.issue_pages(mapping, now)
            if not finished:
                errors.append("github_issue_pagination_pending")
        except FetchError as failure:
            errors.append(str(failure))
        if native_id is None:
            raise FetchError(errors[0] if errors else "github_repository_unbound")
        # Oldest-first attempts prevent a failed/large PR from starving later records.
        rows = self.state.db.execute("SELECT * FROM github_items WHERE project_id=? AND (dirty=1 OR (kind='pull' AND json_extract(detail,'$.state')='open')) ORDER BY last_checked,number", (mapping.project_id,)).fetchall()
        for row in rows:
            if row["kind"] == "pull" and self.api.remaining <= 0:
                errors.append("github_pull_refresh_pending")
                break
            failure = self.task(mapping, native_id, row, now)
            observed += 1
            if failure:
                errors.append(failure)
        scan_status = "unreadable" if coverage == "unreadable" and not observed else "partial" if errors else "complete"
        reason = f"Read-only GitHub {coverage} issue coverage (non-atomic API pages, daily full reconciliation); {observed} positive task observations; no absence deletion"
        payload = {"external_id": f"repo:{native_id}", "name": f"{mapping.owner}/{mapping.name}", "path": f"https://github.com/{mapping.owner}/{mapping.name}",
                   "status": "archived" if repo.get("archived") is True else "active" if repo.get("archived") is False else "unknown", "reason": reason, "collected_at": timestamp(now), "scan_status": scan_status, "task_count": observed}
        if errors:
            payload["scan_error"] = "; ".join(sorted(set(errors)))
        self.state.enqueue(self.config, mapping, "project.observed", payload, now, False)
        report = {"collected_at": timestamp(now), "scan_status": scan_status, "coverage": coverage, "task_count": observed, "scan_error": payload.get("scan_error")}
        self.state.db.execute("INSERT INTO projects(project_id,report,last_success) VALUES(?,?,?) ON CONFLICT(project_id) DO UPDATE SET report=excluded.report,last_success=COALESCE(excluded.last_success,projects.last_success)", (mapping.project_id, canonical(report), timestamp(now) if scan_status == "complete" else None))
        return bool(errors)

    def failed_repository(self, mapping, now, error):
        known = self.state.meta("github_repo:" + mapping.project_id)
        report = {"collected_at": timestamp(now), "scan_status": "unreadable", "coverage": "unverified", "task_count": 0, "scan_error": error}
        self.state.db.execute("INSERT INTO projects(project_id,report) VALUES(?,?) ON CONFLICT(project_id) DO UPDATE SET report=excluded.report", (mapping.project_id, canonical(report)))
        if known:
            payload = {"external_id": f"repo:{known['id']}", "name": known["name"], "path": "https://github.com/" + known["name"], "status": "unknown", "reason": "Repository unavailable or identity unverified; prior tasks retained without refresh", "collected_at": timestamp(now), "scan_status": "unreadable", "task_count": 0, "scan_error": error}
            self.state.enqueue(self.config, mapping, "project.observed", payload, now, False)

    def collect(self, now):
        self.api.begin(now)
        errors = []
        offset = self.state.meta("github_repo_offset", 0) % len(self.config.projects)
        mappings = self.config.projects[offset:] + self.config.projects[:offset]
        for mapping in mappings:
            try:
                with self.state.db:
                    if self.collect_repository(mapping, now):
                        errors.append(mapping.project_id)
            except Exception as failure:
                with self.state.db:
                    error = str(failure) if isinstance(failure, FetchError) else type(failure).__name__
                    self.state.set_meta("github_error:" + mapping.project_id, error)
                    self.failed_repository(mapping, now, error)
                errors.append(mapping.project_id)
        with self.state.db:
            self.state.set_meta("github_repo_offset", (offset + 1) % len(self.config.projects))
            self.state.set_meta("last_collection", timestamp(now))
            self.state.set_meta("collection_error", "github_partial_or_unreadable_scan" if errors else None)
            retry = self.state.meta("github_next_request", 0)
            self.state.set_meta("provider_status", {"source_system": "github", "next_request_at": timestamp(retry) if retry > now else None,
                                                   "requests_last_pass": self.config.request_budget - self.api.remaining,
                                                   "request_budget": self.config.request_budget, "full_reconciliation_seconds": 86400})


def main():
    parser = argparse.ArgumentParser(description=__doc__, epilog="once sends one bounded batch per repository; exit 2 means pending work, exit 1 means collection/delivery error or quarantine. GitHub token files are optional, explicit and externally refreshed.")
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
                collector = Collector(config, state)
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
                        if status["collection_error"] or status["transport_error"] or status["quarantined"] or any(row["delivery_error"] for row in status["projects"]):
                            return 1
                        return 2 if status["pending"] else 0
                    stopped.wait(min(5, max(0.1, next_collection - time.monotonic())))
            finally:
                state.close()
        return 0
    except Exception as failure:
        print(canonical({"error": str(failure) if isinstance(failure, ConnectorError) else type(failure).__name__}), file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
