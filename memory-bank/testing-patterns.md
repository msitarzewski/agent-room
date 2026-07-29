# Testing Patterns

**Status:** Foundational quality contract

**Last updated:** 2026-07-27

## Test Pyramid

### Unit

- command validation and authorization decisions
- event reducers and projections
- task transition rules
- situation correlation and attention deduplication
- evidence verification rules
- budget calculations
- adapter payload translation
- sensitivity and redaction behavior

### Contract

- ARP schema compatibility
- additive-field tolerance
- breaking-version rejection
- idempotency behavior
- source provenance preservation
- Hermes hook, API, and MCP boundary contracts
- Caddy-facing HTTP and WebSocket behavior

### Integration

- command → event → projection
- telemetry → event → situation → attention
- completion claim → evidence → review → verified completion
- adapter reconnect and reconciliation
- approval expiry and exact-action binding
- persistent chat identity without implicit control permission
- database migration and restart recovery

### End to End

- connect a worker and observe real activity
- return after an absence and identify work and blockers
- review completion evidence
- approve, pause, resume, redirect, and cancel authorized work
- Pip joins chat and uses only authorized Agent Room capabilities
- browser traffic through Caddy reaches Agent Room's loopback listener
- public requests cannot reach private or administrative routes

## Determinism

- replay the same event stream into an empty projection store
- compare the complete resulting state
- test duplicate and out-of-order delivery
- freeze time or inject clocks
- inject ID generation
- avoid live network dependencies in deterministic suites

## Adapter Fixtures

Captured native payloads may be used as test fixtures when sanitized.

Fixtures must:

- identify their source version
- remove credentials and personal data
- preserve edge cases relevant to parsing
- never masquerade as production data
- fail clearly when the upstream contract changes

## Security Tests

- anonymous access denial
- role and capability enforcement
- cross-project isolation
- chat-to-command separation
- replayed command rejection
- stale version conflict
- approval scope and expiry
- secret and sensitive-value redaction
- prompt and payload injection boundaries
- request size and rate limits
- private-route exposure through Caddy
- destructive action approval enforcement

## Migration Tests

For every migration:

- migrate a copy of the previous schema
- verify invariants and representative queries
- restart on the migrated database
- validate compatibility with the intended application version
- exercise rollback or backup restore
- prove idempotent migration detection

## Deployment Tests

- verify artifact checksum
- validate configuration before start
- private health and readiness
- public smoke path through co-located Caddy
- WebSocket connection through Caddy
- failed readiness triggers rollback
- last-known-good version restores service
- Hermes remains running and its data unchanged

## Product Validation

Automated tests cannot prove the product mission.

Dogfooding should record:

- connection time for the first worker
- time to understand state after an absence
- terminal polling frequency
- false or unnecessary interruption count
- attention acknowledgement time
- percentage of completion claims with inspectable evidence
- control actions completed without returning to native worker interfaces

Do not optimize these measures with hidden automation or fabricated data.

## Quality Gate

Before production approval:

- all required tests pass
- lint and static analysis have zero errors
- security review is complete
- migrations and restore are tested
- private and public smoke checks pass
- no unresolved critical or high-severity vulnerability is knowingly introduced
- documentation reflects the deployed behavior
