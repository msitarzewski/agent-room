# Shared-host cutover recovery

On November 18, 2026, the destination rejected final provisioning because the required `~/harden-site.sh` was missing. The Mac had already fenced its three writers and uploaded the final snapshot. The destination's provision and restore markers matched the rehearsal, and no activation marker existed.

## Applied changes

Michael approved restoring the host's committed hardener as a regular file at the required path. Its contents were verified against the tracked copy. The approved recovery patch was applied to `feat/shared-host-migration`.

- `deploy/run-migration.py:251` verifies the recorded rehearsal backup before allowing its digest in recovery preflight. The runner checks the uploaded final digest and refuses to reuse the export if a local writer is loaded.
- `deploy/shared-host.sh:508` checks the destination's installation markers, intent, hardener, and stopped services before provisioning can resume. A prior activation blocks provisioning recovery.
- `tests/deployment/test_run_migration.py:313` exercises the retry through the orchestration entry point. `tests/deployment/test_shared_host.py:183` covers interrupted marker transitions and rejects foreign or active state.

## Verification

All 83 deployment tests, 30 GitHub tests, and 23 Sentry tests passed in the deployment checkout. ShellCheck, deployment static checks, completeness scanning, and validation of the real bundle passed. The resumed runner passed live preflight, provisioned the site, restored both final databases, and recorded activation intent. The startup sudo prompt timed out before the remote activation marker was written. The assistant restarted the same command from the restored checkpoint. The private orchestration checkpoint records subsequent startup and health checks.

[Shared-host deployment procedure](../../build-deployment.md#shared-host-migration)

## Cutover outcome

The runner recorded cutover completion and deployment handoff at 18:22:36 CST. The destination passed its health checks, including fresh GitHub delivery. The web page returned HTTP 200 from the Mac. Its launch agents for the SSH tunnel and Sentry publisher were running, and all 99 Sentry projects recorded a successful delivery at 18:22:47 CST. The queue had zero pending or quarantined events and zero project delivery errors. The old local app and GitHub launch agents were unloaded. Browser acceptance precedes local retirement.
