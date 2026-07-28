# Contributing to Agent Room

Thank you for helping improve Agent Room.

## Before You Start

Read [AGENTS.md](AGENTS.md), [boot.md](boot.md), and the relevant Memory Bank
architecture and decision records before proposing a change. Open an issue or
discussion before undertaking a large feature, protocol change, database
migration, or deployment-architecture change.

For security vulnerabilities, follow [SECURITY.md](SECURITY.md) instead of
opening a public issue.

## Development

Use the pinned Go, Node.js, and npm versions and follow the setup in
[README.md](README.md#install-and-verify-the-source-tree). Keep development
data and credentials isolated from production.

Before submitting a pull request, run:

```sh
go install github.com/google/go-licenses@v1.6.0
go test -race ./...
npm --prefix web run typecheck
npm --prefix web run lint
npm --prefix web run api:lint
npm --prefix web test
npm --prefix web run build
tests/security/deployment-static.sh
tests/security/completeness-scan.sh
tests/security/license-compliance.sh
```

Run integration, browser, and authorized security suites when the affected
surface requires them. Do not weaken or skip a gate to make a change pass.

## Pull Requests

- Keep changes focused and explain the user or operator outcome.
- Reuse existing services, patterns, components, and tests where practical.
- Add deterministic tests for new behavior and regression tests for fixes.
- Preserve authorization, audit, project-isolation, and idempotency semantics.
- Do not commit secrets, generated credentials, private topology, runtime data,
  build products, or dependency directories.
- Update public contracts and operational documentation when behavior changes.
- Call out migrations, compatibility effects, security impact, and rollback
  requirements.

By intentionally submitting a contribution for inclusion in Agent Room, you
agree that it is licensed under the Apache License, Version 2.0, as described
in [LICENSE](LICENSE), and represent that you have the right to submit it.
