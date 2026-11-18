#!/usr/bin/env python3
"""Prepare a private, operator-run move from local staging to a shared Linux host.

Never connects to or changes the destination. Snapshot preparation does not stop
the local services. The generated cutover script fences writes before exporting
its final snapshot; remote installation remains an explicit operator action.
"""
from __future__ import annotations

import argparse
import contextlib
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import socket
import sqlite3
import subprocess
import sys
import tarfile
import urllib.parse

SITE = "/srv/www/agentroom"
RUNTIME = "/var/lib/agentroom-shared/github"
PORTS = {"http": 4445, "oidc": 5557, "postgres": 55440,
         "admin": 54490, "adapter": 54491}


class MigrationError(Exception):
    pass


def require(condition: bool, message: str) -> None:
    if not condition:
        raise MigrationError(message)


def config_values(path: Path) -> dict[str, str]:
    result = {}
    for line in path.read_text().splitlines():
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        key, sep, raw = line.partition("=")
        require(bool(sep) and bool(re.fullmatch(r"AGENTROOM_[A-Z_]+", key)),
                "invalid application configuration entry")
        require(key not in result, "duplicate application configuration entry")
        values = shlex.split(raw)
        require(len(values) <= 1, "configuration must contain one literal value")
        result[key] = values[0] if values else ""
    return result


def origin(value: str) -> str:
    parsed = urllib.parse.urlsplit(value)
    require(parsed.scheme == "https" and parsed.hostname is not None and
            parsed.hostname.endswith(".ts.net") and parsed.port == 4445 and
            parsed.username is None and parsed.password is None and
            parsed.path in ("", "/") and not parsed.query and not parsed.fragment,
            "destination must be a private Tailscale HTTPS origin on port 4445")
    require(bool(re.fullmatch(r"[a-zA-Z0-9.-]+", parsed.hostname)),
            "invalid destination hostname")
    return value.rstrip("/")


def identifier(value: str) -> str:
    require(bool(re.fullmatch(r"[a-z_][a-z0-9_]{0,62}", value)),
            "unsafe database identifier")
    return value


def literal(value: str) -> str:
    require("\x00" not in value and "\n" not in value and "\r" not in value,
            "invalid secret value")
    return "'" + value.replace("'", "''") + "'"


def scalar(text: str, key: str) -> str:
    matches = re.findall(r"^\s+(?:-\s+)?" + re.escape(key) + r":\s*(.+?)\s*$", text, re.M)
    require(len(matches) == 1, "identity configuration needs one " + key)
    value = matches[0]
    if value.startswith('"'):
        return json.loads(value)
    if value.startswith("'") and value.endswith("'"):
        return value[1:-1].replace("''", "'")
    require(" #" not in value and not value.startswith(("[", "{", "|", ">")),
            "unsupported identity scalar")
    return value


def write(path: Path, value: str | bytes, executable: bool = False) -> None:
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    require(not path.is_symlink(), "refusing a symbolic link in output")
    if isinstance(value, str):
        value = value.encode()
    with path.open("wb") as stream:
        stream.write(value)
    path.chmod(0o700 if executable else 0o600)


def copy(source: Path, target: Path, executable: bool = False) -> None:
    require(source.is_file() and not source.is_symlink(), "missing regular input file")
    write(target, source.read_bytes(), executable)


def tree(source: Path, target: Path) -> None:
    require(source.is_dir() and not source.is_symlink(), "missing input directory")
    target.mkdir(parents=True, exist_ok=True, mode=0o700)
    for path in sorted(source.rglob("*")):
        require(not path.is_symlink(), "symbolic links are not allowed in migration data")
        if path.is_file():
            copy(path, target / path.relative_to(source), os.access(path, os.X_OK))
        else:
            require(path.is_dir(), "special files are not allowed in migration data")


def sqlite_snapshot(source: Path, target: Path) -> None:
    require(source.is_file() and not source.is_symlink(), "source queue is absent")
    target.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    with contextlib.closing(sqlite3.connect(source.resolve().as_uri() + "?mode=ro", uri=True)) as before:
        with contextlib.closing(sqlite3.connect(target)) as after:
            before.backup(after)
            require(after.execute("PRAGMA integrity_check").fetchone()[0] == "ok",
                    "queue snapshot failed integrity verification")
    target.chmod(0o600)


def manifest(directory: Path) -> dict:
    files = []
    for path in sorted(directory.rglob("*")):
        require(not path.is_symlink(), "migration bundle contains a symbolic link")
        if path.is_file() and path != directory / "manifest.json":
            files.append({"path": path.relative_to(directory).as_posix(),
                          "sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
                          "size": path.stat().st_size})
    return {"schema": 1, "files": files}


def seal(directory: Path) -> None:
    write(directory / "manifest.json", json.dumps(manifest(directory), indent=2) + "\n")


def source_inputs(repo: Path, local: Path) -> dict:
    paths = ["agentroom.conf", "compose.json", "dex-config.yaml", "deployment.json",
             "secrets/database-url", "secrets/session-secret", "secrets/oidc-client-secret",
             "secrets/postgres-admin-password", "secrets/identity-database-password",
             "github/connector.toml", "github/secrets/source-token"]
    result = {}
    for relative in paths:
        path = local / relative
        require(path.is_file() and not path.is_symlink(), "missing regular source configuration input")
        result[relative] = hashlib.sha256(path.read_bytes()).hexdigest()
    for binary in ["agentroomd", "agentroomctl"]:
        result["native/" + binary] = hashlib.sha256((repo / "bin" / binary).read_bytes()).hexdigest()
    for relative in ["adapters/source.py", "adapters/github/agentroom_github.py"]:
        result["collector-code/" + relative] = hashlib.sha256((repo / relative).read_bytes()).hexdigest()
    return {"schema": 1, "files": result}


def export_snapshot(repo: Path, local: Path, bundle: Path, final: bool = False) -> None:
    metadata = json.loads((bundle / "metadata.json").read_text())
    baseline = json.loads((bundle / "source-inputs.json").read_text())
    require(source_inputs(repo, local) == baseline,
            "source configuration or credentials changed; prepare a fresh migration")
    if final:
        for label in ["local.agentroom", "local.agentroom-github", "local.agentroom-sentry"]:
            result = subprocess.run(["launchctl", "print", f"gui/{os.getuid()}/{label}"],
                                    capture_output=True)
            require(result.returncode != 0, "final snapshot requires all local writers unloaded")
        cfg = config_values(local / "agentroom.conf")
        for key in ["AGENTROOM_HTTP_ADDR", "AGENTROOM_ADMIN_ADDR", "AGENTROOM_ADAPTER_ADDR"]:
            host, port = cfg[key].rsplit(":", 1)
            try:
                connection = socket.create_connection((host, int(port)), timeout=0.3)
            except ConnectionRefusedError:
                continue
            else:
                connection.close()
                raise MigrationError("local application listeners must be closed before final snapshot")
        processes = subprocess.run(["ps", "-axo", "command="], capture_output=True, text=True)
        require(processes.returncode == 0, "cannot prove local writer processes stopped")
        for command in processes.stdout.splitlines():
            require(not any(value in command for value in
                    [str(repo / "bin/agentroomd"), "./bin/agentroomd",
                     "adapters/github/agentroom_github.py", "adapters/sentry/agentroom_sentry.py"]),
                    "local writer process remains after unloading")
    compose = ["docker", "compose", "-f", str(local / "compose.json")]
    source_compose = json.loads((local / "compose.json").read_text())
    admin = identifier(source_compose["services"]["postgres"]["environment"]["POSTGRES_USER"])
    for key, name in [("app_database", "app"), ("identity_database", "identity")]:
        target = bundle / "dumps" / (name + ".dump")
        target.parent.mkdir(exist_ok=True, mode=0o700)
        temporary = target.with_suffix(".tmp")
        with temporary.open("wb") as output:
            result = subprocess.run(compose + ["exec", "-T", "postgres", "pg_dump",
                                    "-U", admin, "-Fc", "--no-owner", "--no-acl",
                                    metadata[key]], stdout=output, stderr=subprocess.PIPE)
        require(result.returncode == 0, "database snapshot failed; source was not changed")
        require(temporary.stat().st_size > 0, "empty database snapshot")
        temporary.replace(target)
        target.chmod(0o600)
    source_config = config_values(local / "agentroom.conf")
    artifact_root = Path(source_config["AGENTROOM_ARTIFACT_DIR"])
    if (bundle / "artifacts").exists():
        shutil.rmtree(bundle / "artifacts")
    tree(artifact_root, bundle / "artifacts")
    sqlite_snapshot(local / "github/state/queue.sqlite3", bundle / "github/state/queue.sqlite3")
    require(source_inputs(repo, local) == baseline,
            "source configuration or credentials changed during snapshot; prepare again")
    write(bundle / "snapshot.json", json.dumps({"schema": 1,
          "kind": "final" if final else "rehearsal", "source_fenced": final,
          "created_at": datetime.datetime.now(datetime.timezone.utc).isoformat()}) + "\n")
    seal(bundle)


def unpack_release(archive: Path, directory: Path) -> None:
    # Signature and provenance are checked before preparation and again remotely.
    # Extraction itself must also be safe even when given an untrusted archive.
    with tarfile.open(archive, "r:gz") as source:
        members = source.getmembers()
        for member in members:
            path = Path(member.name)
            require(not path.is_absolute() and ".." not in path.parts and
                    (member.isdir() or member.isfile()), "unsafe release archive member")
        source.extractall(directory, members=members, filter="data")


def prepare(args: argparse.Namespace) -> None:
    repo = Path(__file__).resolve().parents[1]
    local = Path(args.local).resolve()
    bundle = Path(args.output).absolute()
    require(not bundle.exists(), "output already exists; use export to refresh a snapshot")
    require(not any(p.is_symlink() for p in [bundle, *bundle.parents]),
            "output path may not contain symbolic links")
    ignored = subprocess.run(["git", "-C", str(repo), "check-ignore", "--quiet", str(bundle)])
    require(ignored.returncode == 0, "migration output must be inside a Git-ignored directory")
    url = origin(args.url)
    require(bool(re.fullmatch(r"[a-zA-Z0-9][a-zA-Z0-9_.-]*", args.ssh_host)),
            "use a simple SSH host alias without options or shell syntax")
    baseline = source_inputs(repo, local)
    cfg = config_values(local / "agentroom.conf")
    database = urllib.parse.urlsplit((local / "secrets/database-url").read_text().strip())
    app_role = identifier(urllib.parse.unquote(database.username or ""))
    app_database = identifier(database.path.lstrip("/"))
    dex_text = (local / "dex-config.yaml").read_text()
    identity_role = identifier(scalar(dex_text, "user"))
    identity_database = identifier(scalar(dex_text, "database"))
    require(app_database != identity_database and app_role != identity_role,
            "application and identity stores must have separate roles/databases")
    issuer = url.replace(":4445", ":4446") + "/dex"
    archive = Path(args.release).resolve()
    verification = subprocess.run(["cosign", "verify-blob", "--new-bundle-format=false",
                                   "--key", args.public_key, "--signature", str(archive) + ".sig",
                                   str(archive)], capture_output=True, timeout=60)
    require(verification.returncode == 0, "release signature verification failed")
    # Bind to a real previously verified release, never a replacement web build.
    unpack_release(archive, bundle / "release")
    release_manifest = json.loads((bundle / "release/manifest.json").read_text())
    require(release_manifest.get("signed") is True and
            release_manifest.get("target") == "linux/amd64" and
            re.fullmatch(r"[0-9a-f]{40}", release_manifest.get("source_revision", "")),
            "migration requires a signed Linux release with exact source provenance")
    for relative, source in [("payload/release.tar.gz", archive),
                             ("payload/release.tar.gz.sha256", Path(str(archive) + ".sha256")),
                             ("payload/release.tar.gz.sig", Path(str(archive) + ".sig")),
                             ("payload/release-public.pem", Path(args.public_key)),
                             ("tools/cosign", Path(args.linux_cosign)),
                             ("verify-artifact.sh", repo / "deploy/verify-artifact.sh"),
                             ("shared-host.sh", repo / "deploy/shared-host.sh")]:
        copy(source, bundle / relative, relative in ["tools/cosign", "verify-artifact.sh", "shared-host.sh"])
    # The checksum filename must match the renamed private payload.
    digest = hashlib.sha256(archive.read_bytes()).hexdigest()
    write(bundle / "payload/release.tar.gz.sha256", digest + "  release.tar.gz\n")
    source_compose = json.loads((local / "compose.json").read_text())
    admin = identifier(source_compose["services"]["postgres"]["environment"]["POSTGRES_USER"])
    metadata = {"schema": 1, "site": "agentroom", "svcuser": "site_agentroom",
                "source_revision": release_manifest["source_revision"],
                "public_url": url, "issuer": issuer, "ports": PORTS,
                "app_database": app_database, "app_role": app_role,
                "identity_database": identity_database, "identity_role": identity_role,
                "postgres_admin": admin}
    write(bundle / "metadata.json", json.dumps(metadata, indent=2) + "\n")
    for name in ["database-url", "session-secret", "oidc-client-secret", "postgres-admin-password"]:
        copy(local / "secrets" / name, bundle / "secrets" / name)
    target_db = database._replace(netloc=database.netloc.rsplit("@", 1)[0] + "@127.0.0.1:55440")
    write(bundle / "secrets/database-url", urllib.parse.urlunsplit(target_db) + "\n")
    identity_password = (local / "secrets/identity-database-password").read_text().strip()
    dex = {"issuer": issuer, "storage": {"type": "postgres", "config": {
        "host": "postgres", "port": 5432, "database": identity_database,
        "user": identity_role, "password": identity_password, "ssl": {"mode": "disable"}}},
        "web": {"http": "0.0.0.0:5556"}, "enablePasswordDB": True,
        "staticClients": [{"id": cfg["AGENTROOM_OIDC_CLIENT_ID"], "name": "Agent Room",
            "secret": (local / "secrets/oidc-client-secret").read_text().strip(),
            "redirectURIs": [url + "/api/v1/auth/callback"]}],
        "staticPasswords": [{key: scalar(dex_text, key) for key in ["email", "hash", "username", "userID"]}]}
    require(dex["staticPasswords"][0]["hash"].startswith(("$2a$", "$2b$", "$2y$")),
            "identity password must retain its existing bcrypt hash")
    write(bundle / "dex-config.json", json.dumps(dex, indent=2) + "\n")
    compose = {"name": "agentroom-host", "services": {
        "postgres": {"image": source_compose["services"]["postgres"]["image"],
            "restart": "unless-stopped", "environment": {"POSTGRES_USER": admin,
                "POSTGRES_DB": app_database, "POSTGRES_PASSWORD_FILE": "/run/secrets/postgres-password"},
            "secrets": ["postgres-password"], "ports": ["127.0.0.1:55440:5432"],
            "volumes": ["postgres-data:/var/lib/postgresql"],
            "healthcheck": {"test": ["CMD-SHELL", "pg_isready -U " + admin], "interval": "2s", "timeout": "3s", "retries": 30},
            "security_opt": ["no-new-privileges:true"]},
        "dex": {"image": source_compose["services"]["dex"]["image"],
            "restart": "unless-stopped", "command": ["dex", "serve", "/etc/dex/config.json"],
            "ports": ["127.0.0.1:5557:5556"],
            "volumes": ["./dex-config.json:/etc/dex/config.json:ro"],
            "security_opt": ["no-new-privileges:true"], "cap_drop": ["ALL"],
            "read_only": True, "tmpfs": ["/tmp:rw,noexec,nosuid,nodev,size=32m"]}},
        "volumes": {"postgres-data": {"name": "agentroom-host-postgres"}},
        "secrets": {"postgres-password": {"file": "./secrets/postgres-admin-password"}}}
    # The host installer sets the Dex numeric UID/GID to the isolated runtime user.
    write(bundle / "compose.json", json.dumps(compose, indent=2) + "\n")
    cfg.update({"AGENTROOM_HTTP_ADDR": "127.0.0.1:4445", "AGENTROOM_ADMIN_ADDR": "127.0.0.1:54490",
                "AGENTROOM_ADAPTER_ADDR": "127.0.0.1:54491", "AGENTROOM_PUBLIC_URL": url,
                "AGENTROOM_ALLOWED_ORIGINS": url, "AGENTROOM_TRUSTED_PROXY_CIDRS": "127.0.0.1/32",
                "AGENTROOM_OIDC_ISSUER": issuer, "AGENTROOM_OIDC_REDIRECT_URL": url + "/api/v1/auth/callback",
                "AGENTROOM_ARTIFACT_DIR": SITE + "/storage/artifacts", "AGENTROOM_WEB_DIR": SITE + "/current/web",
                "AGENTROOM_WORKSPACE_ROOT": SITE + "/storage/workspaces", "AGENTROOM_DEV": "false",
                "AGENTROOM_DATABASE_URL_FILE": SITE + "/secrets/database-url",
                "AGENTROOM_SESSION_SECRET_FILE": SITE + "/secrets/session-secret",
                "AGENTROOM_OIDC_CLIENT_SECRET_FILE": SITE + "/secrets/oidc-client-secret"})
    write(bundle / "agentroom.conf", "".join(key + "=" + json.dumps(value) + "\n" for key, value in cfg.items()))
    role_sql = []
    for role, password in [(app_role, urllib.parse.unquote(database.password or "")), (identity_role, identity_password)]:
        role_sql.append("DO $$ BEGIN IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname=" + literal(role) + ") THEN CREATE ROLE " + role + " LOGIN; END IF; END $$;\nALTER ROLE " + role + " PASSWORD " + literal(password) + ";\n")
    write(bundle / "restore.sql", "".join(role_sql))
    # Keep provider instance, repository binding, native identities and sequences.
    github_text = (local / "github/connector.toml").read_text()
    for key, value in [("state_dir", RUNTIME + "/state"),
                       ("ingest_url", "http://127.0.0.1:54491/api/v1/ingest"),
                       ("token_file", RUNTIME + "/source-token")]:
        github_text, replacements = re.subn(r"^" + key + r"\s*=.*$", lambda m: key + " = " + json.dumps(value), github_text, flags=re.M)
        require(replacements == 1, "expected one explicit GitHub " + key)
    write(bundle / "github/connector.toml", github_text)
    copy(local / "github/secrets/source-token", bundle / "github/source-token")
    for relative in ["adapters/source.py", "adapters/github/agentroom_github.py"]:
        copy(repo / relative, bundle / relative)
    write(bundle / "github-run.sh", "#!/bin/sh\nset -eu\nexec /usr/bin/python3 -B " + SITE +
          "/adapters/github/agentroom_github.py --config " + RUNTIME + "/connector.toml watch\n", True)
    supervisor = []
    for name, command in [("web", SITE + "/current/bin/agentroomd --config " + SITE + "/agentroom.conf"),
                          ("github", SITE + "/github-run.sh")]:
        supervisor.append(f"[program:agentroom-{name}]\ncommand={command}\ndirectory={SITE}\nuser=site_agentroom\n"
                          "autostart=false\nautorestart=true\nstartsecs=3\nstartretries=60\nstopsignal=TERM\nstopasgroup=true\nkillasgroup=true\n"
                          "umask=0077\nredirect_stderr=true\n"
                          f"stdout_logfile={SITE}/storage/logs/{name}.log\nstdout_logfile_maxbytes=10MB\nstdout_logfile_backups=3\n\n")
    write(bundle / "supervisor.conf", "".join(supervisor))
    write(bundle / "operator.json", json.dumps({"ssh_host": args.ssh_host, "repo": str(repo), "local": str(local)}, indent=2) + "\n")
    require(source_inputs(repo, local) == baseline,
            "source configuration changed during preparation; prepare again")
    write(bundle / "source-inputs.json", json.dumps(baseline, indent=2) + "\n")
    export_snapshot(repo, local, bundle)
    print("Private migration bundle prepared. Local services remain unchanged.")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    prep = sub.add_parser("prepare")
    for name in ["local", "output", "url", "ssh-host", "release", "public-key", "linux-cosign"]:
        prep.add_argument("--" + name, required=True)
    export = sub.add_parser("export")
    export.add_argument("--bundle", required=True)
    export.add_argument("--final", action="store_true",
                        help="requires local application and both publishers unloaded")
    args = parser.parse_args()
    try:
        os.umask(0o077)
        if args.command == "prepare":
            prepare(args)
        else:
            bundle = Path(args.bundle).resolve()
            operator = json.loads((bundle / "operator.json").read_text())
            export_snapshot(Path(operator["repo"]), Path(operator["local"]), bundle, args.final)
            print("Private snapshot refreshed; destination was not contacted.")
        return 0
    except (MigrationError, OSError, ValueError, KeyError, sqlite3.Error, tarfile.TarError):
        print("Migration preparation failed; inspect private inputs. No destination was changed.", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
