# 270726_open-source-publication-readiness

## Objective

Prepare Agent Room for public open-source distribution without exposing
credentials or personal infrastructure details, and ensure source and binary
redistribution preserve project and dependency license obligations.

## Outcome

- ✅ Agent Room licensed under Apache License 2.0
- ✅ Copyright assigned to Michael Sitarzewski
- ✅ OpenAPI metadata changed from proprietary to Apache-2.0
- ✅ Contribution and private vulnerability-reporting policies added
- ✅ Runtime dependency licenses reviewed; no GPL, AGPL, SSPL, or incompatible
  production dependency found
- ✅ Required CoreOS notice and MCP transitional license preserved
- ✅ Full Go and browser runtime license texts included under `third_party/`
- ✅ Release archives fail closed when license material is absent
- ✅ Release archives include `LICENSE`, `NOTICE`, `THIRD_PARTY_NOTICES.md`,
  full runtime license texts, manifest, and SPDX SBOM
- ✅ CI enforces the allowed runtime-license policy and exact checked-in Go
  license inventory
- ✅ Personal workstation paths, private deployment topology, physical hostnames, and
  personal test fixtures removed
- ✅ Public deployment examples now use `host_agentroom` and `host_ingress`
  while preserving Pip / `agent_pip` as the Hermes participant
- ✅ No genuine secrets found
- ✅ Public repository created at
  `https://github.com/msitarzewski/agent-room`
- ✅ Canonical Go module and repository identity set to
  `github.com/msitarzewski/agent-room`
- ✅ Initial Git history scanned with gitleaks before publication

## QA

- Go unit suite: pass
- Go race suite: pass
- Frontend typecheck, lint, strict OpenAPI, build: pass
- Frontend unit/component tests: 47/47 pass with coverage thresholds
- Native Chromium/WebKit browser tests: 12/12 pass
- Linux/amd64 cross-build: pass
- Linux release packaging: byte-for-byte reproducible with license contents
  asserted
- Gitleaks: no findings
- Semgrep: no findings
- Gosec: no findings
- Staticcheck: pass
- govulncheck: no reachable vulnerabilities
- npm audit: no vulnerabilities
- Go module verification: pass
- ShellCheck warning/error gate: pass
- Deployment static gate: pass
- Completeness and TODO/stub scans: pass
- Privacy and proprietary-license residue scans: pass
- Diff whitespace check: pass

The full three-engine Linux browser, real OIDC, PostgreSQL integration, adapter
turn, and active ZAP gates remain covered by the immediately preceding founding
release. This patch changes legal/release metadata, documentation, deployment
example names, and test-fixture identities; native browser regression covers
the affected UI fixture.

## Files and Integration

- `LICENSE`, `NOTICE` — project license and persistent attribution
- `SECURITY.md` — private vulnerability disclosure policy
- `CONTRIBUTING.md` — contribution, testing, and inbound-license policy
- `THIRD_PARTY_NOTICES.md`, `third_party/` — reviewed runtime attribution and
  full redistributed license texts
- `api/openapi/agent-room.v1.yaml` — Apache-2.0 API metadata
- `deploy/package-release.sh` — fail-closed legal-material packaging
- `tests/security/license-compliance.sh` — pinned runtime-license policy and
  checked-in license inventory verification
- `.github/workflows/security.yml` — CI license gate
- `boot.md`, `README.md`, `deploy/README.md`, `memory-bank/*` — public
  positioning and generic deployment topology
- `deploy/caddy/*`, `deploy/config/*`, `deploy/firewall/*`,
  `deploy/pgbackrest/*` — generic public deployment identifiers
- `web/src/test/fixtures.ts`, `web/tests/browser/control-room.spec.ts` —
  non-personal operator fixtures

## Architectural Decision

Apache-2.0 was selected over MIT for its explicit patent grant while retaining
permissive commercial and private use. AGPL and proprietary/source-available
terms were rejected because they do not match the adoption goal.

See `memory-bank/decisions.md#2026-07-27-apache-license-20`.

## Remaining Hosting Work

- confirm ownership or replace the `agentroom.dev` namespace before a public
  tagged release
- enable private vulnerability reporting, secret scanning, dependency updates,
  branch protection, required checks, and review rules on the hosting platform
