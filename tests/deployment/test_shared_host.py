"""Portable shared-host boundary tests; all data and shell stand-ins are temporary.

Reuse analysis: test_migration.py covers bundle preparation/export, while these
tests exercise the host shell boundary without a Linux host, sudo or deployment.
Its existing write/seal helpers are reused for self-contained validation fixtures.
"""
import copy
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "deploy/shared-host.sh"
SPEC = importlib.util.spec_from_file_location("shared_host_preparation", ROOT / "deploy/prepare-migration.py")
migration = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(migration)
PUBLIC = "https://node.tail-example.ts.net:4445"
SITE = "/srv/www/agentroom"
RUNTIME = "/var/lib/agentroom-shared/github"


def fixture(root):
    """Only validation is exercised: payload/binary placeholders are never run."""
    metadata = {"schema": 1, "site": "agentroom", "svcuser": "site_agentroom",
                "source_revision": "a" * 40, "public_url": PUBLIC,
                "issuer": PUBLIC.replace(":4445", ":4446") + "/dex",
                "ports": {"http": 4445, "oidc": 5557, "postgres": 55440,
                          "admin": 54490, "adapter": 54491},
                "app_database": "fixture_app_db", "identity_database": "fixture_identity_db",
                "app_role": "fixture_app", "identity_role": "fixture_identity",
                "postgres_admin": "fixture_admin"}
    config = {"AGENTROOM_HTTP_ADDR": "127.0.0.1:4445", "AGENTROOM_ADMIN_ADDR": "127.0.0.1:54490",
              "AGENTROOM_ADAPTER_ADDR": "127.0.0.1:54491", "AGENTROOM_PUBLIC_URL": PUBLIC,
              "AGENTROOM_ALLOWED_ORIGINS": PUBLIC, "AGENTROOM_OIDC_ISSUER": metadata["issuer"],
              "AGENTROOM_OIDC_CLIENT_ID": "fixture-client",
              "AGENTROOM_OIDC_REDIRECT_URL": PUBLIC + "/api/v1/auth/callback",
              "AGENTROOM_DEV": "false", "AGENTROOM_TRUSTED_PROXY_CIDRS": "127.0.0.1/32",
              "AGENTROOM_ARTIFACT_DIR": SITE + "/storage/artifacts",
              "AGENTROOM_WEB_DIR": SITE + "/current/web",
              "AGENTROOM_WORKSPACE_ROOT": SITE + "/storage/workspaces",
              "AGENTROOM_DATABASE_URL_FILE": SITE + "/secrets/database-url",
              "AGENTROOM_SESSION_SECRET_FILE": SITE + "/secrets/session-secret",
              "AGENTROOM_OIDC_CLIENT_SECRET_FILE": SITE + "/secrets/oidc-client-secret"}
    compose = {"name": "agentroom-host", "services": {
        "postgres": {"image": "postgres@sha256:" + "a" * 64,
                     "environment": {"POSTGRES_USER": "fixture_admin", "POSTGRES_DB": "fixture_app_db",
                                     "POSTGRES_PASSWORD_FILE": "/run/secrets/postgres-password"},
                     "secrets": ["postgres-password"], "ports": ["127.0.0.1:55440:5432"],
                     "volumes": ["postgres-data:/var/lib/postgresql"],
                     "security_opt": ["no-new-privileges:true"],
                     "healthcheck": {"test": ["CMD-SHELL", "pg_isready -U fixture_admin"],
                                     "interval": "2s", "timeout": "3s", "retries": 30}},
        "dex": {"image": "ghcr.io/dexidp/dex@sha256:" + "b" * 64,
                "ports": ["127.0.0.1:5557:5556"], "volumes": ["./dex-config.json:/etc/dex/config.json:ro"],
                "command": ["dex", "serve", "/etc/dex/config.json"], "read_only": True,
                "cap_drop": ["ALL"], "security_opt": ["no-new-privileges:true"],
                "tmpfs": ["/tmp:rw,noexec,nosuid,nodev,size=32m"]}},
        "volumes": {"postgres-data": {"name": "agentroom-host-postgres"}},
        "secrets": {"postgres-password": {"file": "./secrets/postgres-admin-password"}}}
    identity = {"issuer": metadata["issuer"], "web": {"http": "0.0.0.0:5556"},
                "storage": {"type": "postgres", "config": {"host": "postgres", "port": 5432,
                            "database": "fixture_identity_db", "user": "fixture_identity",
                            "ssl": {"mode": "disable"}}},
                "staticClients": [{"id": "fixture-client", "secret": "fixture-not-a-credential",
                                   "redirectURIs": [PUBLIC + "/api/v1/auth/callback"]}],
                "staticPasswords": [{"userID": "fixture-original-subject", "hash": "$2b$fixture"}]}
    for name in ["release/bin/agentroomd", "release/bin/agentroomctl", "release/web/index.html",
                 "release/manifest.json", "tools/cosign", "verify-artifact.sh", "payload/release.tar.gz",
                 "payload/release.tar.gz.sha256", "payload/release.tar.gz.sig", "payload/release-public.pem",
                 "github/source-token", "github/state/queue.sqlite3", "adapters/source.py",
                 "adapters/github/agentroom_github.py", "restore.sql", "dumps/app.dump", "dumps/identity.dump",
                 "secrets/session-secret", "secrets/postgres-admin-password"]:
        migration.write(root / name, "validation-fixture-only\n")
    for name, data in [("metadata.json", metadata), ("compose.json", compose), ("dex-config.json", identity),
                       ("snapshot.json", {"schema": 1, "kind": "rehearsal", "source_fenced": False,
                                          "created_at": "2026-10-20T12:00:00Z"})]:
        migration.write(root / name, json.dumps(data))
    migration.write(root / "agentroom.conf", "".join(key + "=" + json.dumps(value) + "\n" for key, value in config.items()))
    migration.write(root / "secrets/oidc-client-secret", "fixture-not-a-credential")
    migration.write(root / "secrets/database-url", "postgres://fixture_app:fixture@127.0.0.1:55440/fixture_app_db")
    migration.write(root / "github/connector.toml", '[connector]\nstate_dir="' + RUNTIME + '/state"\ningest_url="http://127.0.0.1:54491/api/v1/ingest"\n[[repositories]]\ntoken_file="' + RUNTIME + '/source-token"\n')
    migration.write(root / "github-run.sh", "#!/bin/sh\nset -eu\nexec /usr/bin/python3 -B " + SITE + "/adapters/github/agentroom_github.py --config " + RUNTIME + "/connector.toml watch\n")
    supervisor = ""
    for name, command in [("web", SITE + "/current/bin/agentroomd --config " + SITE + "/agentroom.conf"), ("github", SITE + "/github-run.sh")]:
        supervisor += (f"[program:agentroom-{name}]\ncommand={command}\ndirectory={SITE}\nuser=site_agentroom\n"
                       "autostart=false\nautorestart=true\nstartsecs=3\nstartretries=60\nstopsignal=TERM\nstopasgroup=true\nkillasgroup=true\n"
                       "umask=0077\nredirect_stderr=true\n" + f"stdout_logfile={SITE}/storage/logs/{name}.log\n"
                       "stdout_logfile_maxbytes=10MB\nstdout_logfile_backups=3\n\n")
    migration.write(root / "supervisor.conf", supervisor)
    migration.seal(root)


class BundleTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.bundle = Path(self.temporary.name).resolve() / "bundle"
        self.bundle.mkdir(mode=0o700)
        fixture(self.bundle)

    def change(self, name, value, reseal=False):
        (self.bundle / name).write_text(value)
        if reseal:
            migration.seal(self.bundle)

    def jsonchange(self, name, update):
        value = json.loads((self.bundle / name).read_text())
        update(value)
        self.change(name, json.dumps(value), True)

    def runmode(self, mode="validate", bundle=None):
        return subprocess.run(["bash", str(SCRIPT), mode, str(bundle or self.bundle)], capture_output=True)

    def reject(self):
        self.assertNotEqual(self.runmode().returncode, 0)

    def test_valid_complete_bundle(self):
        result = self.runmode()
        self.assertEqual(result.returncode, 0, result.stderr.decode())

    def test_preflight_rejects_different_uploaded_digest_before_host_access(self):
        result = subprocess.run(["bash", str(SCRIPT), "preflight", str(self.bundle), "b" * 64], capture_output=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(b"staged manifest differs", result.stderr)

    def test_preflight_requires_valid_expected_digest(self):
        for value in ["invalid", "", "a" * 63]:
            with self.subTest(value=value):
                result = subprocess.run(["bash", str(SCRIPT), "preflight", str(self.bundle), value], capture_output=True)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(b"staged manifest differs", result.stderr)

    def test_reviewed_helper_can_validate_inline_without_uploading_a_new_file(self):
        code = SCRIPT.read_text() + '\nmain "$@"\n'
        result = subprocess.run(["bash", "-c", code, "agentroom-preflight", "validate", str(self.bundle)], capture_output=True)
        self.assertEqual(result.returncode, 0, result.stderr.decode())

    def test_preflight_requires_owned_staged_markers_and_allows_exact_active_retry(self):
        digest = hashlib.sha256((self.bundle / "manifest.json").read_bytes()).hexdigest()
        code = '''source "$SCRIPT"
SITE="$SANDBOX/site"; HOME="$SANDBOX/home"
mkdir -p "$SITE" "$HOME"
touch "$HOME/harden-site.sh"
uname() { if [[ "$1" == -s ]]; then echo Linux; else echo x86_64; fi; }
id() { if [[ "$1" == -un ]]; then echo michael; else echo 1000; fi; }
verify_release() { :; }; validate_operator_files() { :; }; flock() { :; }; preflight() { :; }
marker() { [[ "$1" != "$MISSING" ]]; }
if [[ "$ACTIVE" == yes ]]; then touch "$SITE/.shared-host-activated"; fi
main preflight "$FIXTURE" "$DIGEST"
'''
        for missing, active, allowed in [("none", "no", True), ("restored", "no", False), ("provisioned", "no", False), ("none", "yes", True), ("activated", "yes", False)]:
            with self.subTest(missing=missing, active=active), tempfile.TemporaryDirectory() as temporary:
                env = os.environ | {"SCRIPT": str(SCRIPT), "SANDBOX": temporary, "FIXTURE": str(self.bundle), "DIGEST": digest, "MISSING": missing, "ACTIVE": active}
                result = subprocess.run(["bash", "-c", code], env=env, capture_output=True)
                self.assertEqual(result.returncode == 0, allowed, result.stderr.decode())

    def test_hardener_is_required_before_fencing_and_on_recovery(self):
        code = '''source "$SCRIPT"
HOME="$SANDBOX"; SITE="$SANDBOX"; BUNDLE="$FIXTURE"; BUNDLE_DIGEST=final
marker() { return 0; }; provision_intent() { return 0; }; require_stopped() { return 0; }
check_preflight_markers "$STAGED"
'''
        for hardener in ("absent", "symlink", "regular"):
            for staged in ("", "stage"):
                with self.subTest(hardener=hardener, staged=staged), tempfile.TemporaryDirectory() as tmp:
                    path = Path(tmp) / "harden-site.sh"
                    if hardener == "regular":
                        path.touch()
                    elif hardener == "symlink":
                        path.symlink_to(SCRIPT)
                    env = os.environ | {"SCRIPT": str(SCRIPT), "SANDBOX": tmp, "FIXTURE": str(self.bundle), "STAGED": staged}
                    result = subprocess.run(["bash", "-c", code], env=env, capture_output=True)
                    self.assertEqual(result.returncode == 0, hardener == "regular", result.stderr.decode())
                    if hardener != "regular":
                        self.assertIn(b"canonical ~/harden-site.sh is required", result.stderr)

    def test_final_upload_recovery_accepts_only_known_inactive_marker_transitions(self):
        self.jsonchange("snapshot.json", lambda m: m.update(kind="final", source_fenced=True))
        final = hashlib.sha256((self.bundle / "manifest.json").read_bytes()).hexdigest()
        stage, foreign = "a" * 64, "b" * 64
        code = '''source "$SCRIPT"
HOME="$SANDBOX"; SITE="$SANDBOX"
uname() { if [[ "$1" == -s ]]; then echo Linux; else echo x86_64; fi; }
id() { if [[ "$1" == -un ]]; then echo michael; else echo 1000; fi; }
verify_release() { :; }; validate_operator_files() { :; }; flock() { :; }; preflight() { :; }
require_stopped() { [[ "$RUNNING" == no ]] || fail "writer active"; }
main preflight "$FIXTURE" "$FINAL" "$STAGED"
'''
        for provisioned, restored, intent, active, running, allowed in (
            (stage, stage, stage, None, False, True), (stage, stage, final, None, False, True),
            (final, stage, final, None, False, True), (final, None, final, None, False, True),
            (final, final, final, None, False, True), (foreign, stage, stage, None, False, False),
            (stage, foreign, stage, None, False, False), (stage, None, stage, None, False, False),
            (None, stage, stage, None, False, False), (stage, stage, foreign, None, False, False),
            (final, None, stage, None, False, False), (final, foreign, final, None, False, False),
            (final, final, final, final, False, False), (stage, stage, stage, foreign, False, False),
            (stage, stage, stage, None, True, False),
        ):
            with self.subTest(provisioned=provisioned, restored=restored, intent=intent, active=active, running=running), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                (root / "harden-site.sh").touch()
                for name, value in ((".shared-host-provisioned", provisioned), (".shared-host-restored", restored),
                                    (".agentroom-shared-host-intent", intent), (".shared-host-activated", active)):
                    if value is not None:
                        (root / name).write_text(value + "\n")
                env = os.environ | {"SCRIPT": str(SCRIPT), "SANDBOX": tmp, "FIXTURE": str(self.bundle),
                                    "FINAL": final, "STAGED": stage, "RUNNING": "yes" if running else "no"}
                result = subprocess.run(["bash", "-c", code], env=env, capture_output=True)
                self.assertEqual(result.returncode == 0, allowed, result.stderr.decode())

    def test_recovery_rejects_rehearsal_upload_and_invalid_predecessor(self):
        digest = hashlib.sha256((self.bundle / "manifest.json").read_bytes()).hexdigest()
        for predecessor, message in (("a" * 64, b"fenced final upload"), (digest, b"invalid rehearsal"), ("", b"invalid rehearsal")):
            with self.subTest(predecessor=predecessor):
                result = subprocess.run(["bash", str(SCRIPT), "preflight", str(self.bundle), digest, predecessor], capture_output=True)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(message, result.stderr)

    def test_digest_mismatch(self):
        self.change("github-run.sh", "changed")
        self.reject()

    def test_extra_file(self):
        (self.bundle / "extra").write_text("extra")
        self.reject()

    def test_missing_file(self):
        (self.bundle / "github/source-token").unlink()
        self.reject()

    def test_symlink(self):
        (self.bundle / "link").symlink_to(self.bundle / "manifest.json")
        self.reject()

    def test_hardlink(self):
        os.link(self.bundle / "github-run.sh", self.bundle / "hardlink")
        self.reject()

    def test_world_write(self):
        (self.bundle / "github-run.sh").chmod(0o777)
        self.reject()

    def test_unsafe_manifest_path(self):
        value = json.loads((self.bundle / "manifest.json").read_text())
        value["files"].append({"path": "../outside", "size": 0, "sha256": "0" * 64})
        self.change("manifest.json", json.dumps(value))
        self.reject()

    def test_duplicate_json_key(self):
        self.change("metadata.json", '{"schema":1,"schema":1}', True)
        self.reject()

    def test_compose_host_network(self):
        self.jsonchange("compose.json", lambda m: m["services"]["postgres"].update({"network_mode": "host"}))
        self.reject()

    def test_compose_foreign_volume(self):
        self.jsonchange("compose.json", lambda m: m["volumes"]["postgres-data"].update({"name": "neighbor"}))
        self.reject()

    def test_compose_external_binding(self):
        self.jsonchange("compose.json", lambda m: m["services"]["dex"].update({"ports": ["0.0.0.0:5557:5556"]}))
        self.reject()

    def test_supervisor_other_site(self):
        self.change("supervisor.conf", (self.bundle / "supervisor.conf").read_text() + "\n[program:neighbor]\ncommand=/bin/true\n", True)
        self.reject()

    def test_supervisor_requires_boot_dependency_retry_budget(self):
        self.change("supervisor.conf", (self.bundle / "supervisor.conf").read_text().replace("startretries=60", "startretries=3"), True)
        self.reject()

    def test_app_runner(self):
        self.change("agentroom.conf", (self.bundle / "agentroom.conf").read_text() + 'AGENTROOM_CODEX_BIN="/usr/bin/codex"\n', True)
        self.reject()

    def test_secret_path_escape(self):
        self.change("agentroom.conf", (self.bundle / "agentroom.conf").read_text().replace(SITE + "/secrets/session-secret", "/etc/neighbor"), True)
        self.reject()

    def test_collector_ingest_escape(self):
        self.change("github/connector.toml", (self.bundle / "github/connector.toml").read_text().replace("127.0.0.1", "0.0.0.0"), True)
        self.reject()

    def test_snapshot_unfenced_final(self):
        self.jsonchange("snapshot.json", lambda m: m.update({"kind": "final", "source_fenced": False}))
        self.reject()

    def test_rehearsal_start_rejected_before_verifier(self):
        result = self.runmode("start")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(b"final snapshot", result.stderr)

    def test_relative_bundle(self):
        self.assertNotEqual(self.runmode(bundle="relative").returncode, 0)

    def test_parent_symlink(self):
        link = self.bundle.parent / "link"
        link.symlink_to(self.bundle)
        self.assertNotEqual(self.runmode(bundle=link).returncode, 0)


class ShellTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.sandbox = Path(self.temporary.name).resolve()

    def shell(self, code, extra=None, data=None):
        environment = os.environ | {"SCRIPT": str(SCRIPT), "SANDBOX": str(self.sandbox)} | (extra or {})
        return subprocess.run(["bash", "-c", 'source "$SCRIPT"\n' + code], env=environment,
                              input=data, capture_output=True)

    def test_operator_state_rejects_symlink_and_unrelated_lock_content(self):
        sentinel = self.sandbox / "sentinel"
        sentinel.write_text("untouched")
        lock = self.sandbox / ".agentroom-shared-host.lock"
        lock.symlink_to(sentinel)
        self.assertNotEqual(self.shell('HOME="$SANDBOX"; validate_operator_files').returncode, 0)
        lock.unlink()
        lock.write_text("unrelated data")
        lock.chmod(0o600)
        self.assertNotEqual(self.shell('HOME="$SANDBOX"; validate_operator_files').returncode, 0)
        self.assertEqual(sentinel.read_text(), "untouched")
        self.assertEqual(lock.read_text(), "unrelated data")
        lock.write_text("")
        self.assertEqual(self.shell('HOME="$SANDBOX"; validate_operator_files').returncode, 0)

    def test_collection_time_is_parsed_and_delivery_is_required(self):
        for observed, delivery, wanted in [
            ("2026-10-20T12:00:00.123456Z", "2026-10-20T12:00:00.234567Z", True),
            ("2026-10-20T11:59:59.999999Z", "2026-10-20T12:00:01Z", False),
            ("2026-10-20T12:00:01Z", "2026-10-20T11:59:59Z", False),
            ("malformed", "2026-10-20T12:00:01Z", False)]:
            status = {"initialized": True, "last_collection": observed, "pending": 0,
                      "quarantined": 0, "projects": [{"last_delivery": delivery, "delivery_error": None}]}
            result = self.shell("collector_healthy 2026-10-20T12:00:00Z", data=json.dumps(status).encode())
            self.assertEqual(result.returncode == 0, wanted)

    def test_exact_owned_routes_and_neighbor_isolation(self):
        host, port = "node.tail-example.ts.net", "4445"
        exact = {"TCP": {port: {"HTTPS": True}}, "Web": {host + ":" + port: {"Handlers": {"/": {"Proxy": "http://127.0.0.1:4445"}}}}}
        foreign = copy.deepcopy(exact)
        foreign["Web"][host + ":" + port]["Handlers"]["/"]["Proxy"] = "http://127.0.0.1:9999"
        foreground = copy.deepcopy(exact)
        foreground["Foreground"] = {"session": {"TCP": {port: {"HTTPS": True}}}}
        funnel = copy.deepcopy(exact)
        funnel["AllowFunnel"] = {host + ":" + port: True}
        for config, owned, wanted in [({}, False, True), (exact, False, False), (exact, True, True),
                                      (foreign, True, False), (foreground, True, False), (funnel, True, False),
                                      ({"TCP": {"4447": {"HTTPS": True}}}, False, True)]:
            marker = self.sandbox / ".shared-host-serve-4445"
            if owned:
                marker.write_text(PUBLIC)
            elif marker.exists():
                marker.unlink()
            (self.sandbox / "fixture.json").write_text(json.dumps(config))
            result = self.shell('SITE="$SANDBOX"; PUBLIC_URL=' + PUBLIC + '; TAILNET_HOST=' + host + '\ntailscale() { cat "$SANDBOX/fixture.json"; }\nserve_state 4445 http://127.0.0.1:4445')
            self.assertEqual(result.returncode == 0, wanted, result.stderr.decode())

    def test_partial_provision_requires_exact_intent(self):
        code = '''SITE="$SANDBOX/site"; RUNTIME="$SANDBOX/runtime"; SUPERVISOR="$SANDBOX/supervisor.conf"; HOME="$SANDBOX/home"
mkdir -p "$HOME" "$SITE" "$RUNTIME"
BUNDLE_DIGEST=abc; PUBLIC_URL=https://node.tail-example.ts.net:4445
HTTP_PORT=4445; OIDC_PORT=5557; PG_PORT=55440; ADMIN_PORT=54490; ADAPTER_PORT=54491
uname() { if [[ "$1" == -s ]]; then echo Linux; else echo x86_64; fi; }
id() { case "$1" in -un) echo michael;; -u) echo 1000;; *) return 1;; esac; }
flock() { :; }; docker() { :; }; sudo() { :; }; supervisorctl() { :; }; ss() { :; }; getent() { return 1; }
tailscale() { printf '%s' '{"Self":{"DNSName":"node.tail-example.ts.net."}}'; }
safe_host_paths() { :; }; check_cluster_scope() { :; }; check_ports() { :; }; serve_state() { echo absent; }
if [[ "$INTENT" == exact ]]; then echo abc > "$HOME/.agentroom-shared-host-intent"; elif [[ "$INTENT" == foreign ]]; then echo other > "$HOME/.agentroom-shared-host-intent"; fi
preflight
'''
        for intent, wanted in [("none", False), ("foreign", False), ("exact", True)]:
            result = self.shell(code, {"INTENT": intent})
            self.assertEqual(result.returncode == 0, wanted, result.stderr.decode())

    def test_partial_release_resumes_but_completed_release_never_changes(self):
        code = '''SITE="$SANDBOX/$CORRUPT/site"; BUNDLE="$SANDBOX/$CORRUPT/bundle"; HOME="$SANDBOX/$CORRUPT/home"
REVISION=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa; BUNDLE_DIGEST=abc
mkdir -p "$HOME" "$BUNDLE/release/web" "$SITE/releases/$REVISION"
echo abc > "$HOME/.agentroom-shared-host-intent"
echo signed > "$BUNDLE/release/manifest.json"; echo complete > "$BUNDLE/release/web/index.html"
cp "$BUNDLE/release/manifest.json" "$SITE/releases/$REVISION/manifest.json"
install_release
[[ -f "$SITE/releases/$REVISION/web/index.html" && -f "$SITE/.shared-host-release-$REVISION" ]]
if [[ "$CORRUPT" == yes ]]; then echo changed > "$SITE/releases/$REVISION/web/index.html"; fi
install_release
'''
        for corrupt, wanted in [("no", True), ("yes", False)]:
            result = self.shell(code, {"CORRUPT": corrupt})
            self.assertEqual(result.returncode == 0, wanted, result.stderr.decode())

    def test_restore_activated_never_touches_database(self):
        result = self.shell('SITE="$SANDBOX"; touch "$SITE/.shared-host-activated"; compose() { touch "$SITE/TOUCHED"; }; restore')
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.sandbox / "TOUCHED").exists())

    def test_backup_site_verifies_dumps_and_writes_manifest(self):
        code = '''SITE="$SANDBOX/site"; RUNTIME="$SANDBOX/runtime"; OUT="$SANDBOX/backups"
mkdir -p "$SITE/storage/artifacts"
touch "$SITE/.shared-host-activated"
ln -sfn "releases/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" "$SITE/current"
printf '{"services":{"postgres":{"environment":{"POSTGRES_USER":"pgadmin","POSTGRES_DB":"appdb"}}}}' > "$SITE/compose.json"
printf '{"storage":{"config":{"database":"iddb"}}}' > "$SITE/dex-config.json"
printf 'artifact-bytes' > "$SITE/storage/artifacts/sample.txt"
compose() {
  case "$*" in
    *pg_dump*) printf 'PGDMP-fixture'; return 0 ;;
    *pg_restore*) cat >/dev/null; return 0 ;;
  esac
  return 1
}
sudo() { return 1; }
backup_site "$OUT"
'''
        result = self.shell(code)
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        snapshots = list((self.sandbox / "backups").glob("agentroom-backup-*"))
        self.assertEqual(len(snapshots), 1)
        manifest = json.loads((snapshots[0] / "manifest.json").read_text())
        self.assertEqual(manifest["databases"], {"app": "appdb", "identity": "iddb"})
        self.assertEqual({entry["path"] for entry in manifest["files"]}, {"app.dump", "identity.dump", "artifacts.tar.gz"})

    def test_upgrade_release_swaps_current_and_rolls_back_on_readiness_failure(self):
        code = '''SITE="$SANDBOX/site"; ARTIFACT="$SANDBOX/release.tar.gz"; PUBKEY="$SANDBOX/key.pem"; VERIFIER="$SANDBOX/verify.sh"
mkdir -p "$SITE/releases/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
touch "$SITE/.shared-host-activated" "$ARTIFACT" "$PUBKEY"
ln -sfn "releases/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" "$SITE/current"
printf 'AGENTROOM_ADMIN_ADDR="127.0.0.1:54490"\n' > "$SITE/agentroom.conf"
cat > "$VERIFIER" <<'VSH'
#!/usr/bin/env bash
out=""
while (($#)); do
  if [[ "$1" == "--extract-to" ]]; then out="$2"; shift 2; else shift; fi
done
mkdir -p "$out/bin" "$out/web"
printf '{"source_revision":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}\n' > "$out/manifest.json"
printf '#!/bin/sh\nexit 0\n' > "$out/bin/agentroomctl"
chmod +x "$out/bin/agentroomctl"
printf 'bin' > "$out/bin/agentroomd"
printf '<html></html>' > "$out/web/index.html"
VSH
chmod +x "$VERIFIER"
sudo() {
  if [[ "$1" == "-u" ]]; then shift 2; "$@"; return $?; fi
  return 0
}
wait_status() { [[ "$READY" == ok ]]; }
upgrade_release "$ARTIFACT" "$PUBKEY" "$VERIFIER"
'''
        ok = self.shell(code, {"READY": "ok"})
        self.assertEqual(ok.returncode, 0, ok.stderr.decode())
        self.assertEqual(os.readlink(self.sandbox / "site/current"), "releases/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

        # Reset and verify automatic rollback when readiness fails
        os.unlink(self.sandbox / "site/current")
        os.symlink("releases/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", self.sandbox / "site/current")
        fail_res = self.shell(code, {"READY": "bad"})
        self.assertNotEqual(fail_res.returncode, 0)
        self.assertEqual(os.readlink(self.sandbox / "site/current"), "releases/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")


if __name__ == "__main__":
    unittest.main()
