# Project Brief

**Project:** Agent Room

**Status:** Product foundation

**Source of vision:** `boot.md`

## Mission

Agent Room gives people one trusted place to understand what their AI workers are doing, identify what needs attention, inspect the supporting evidence, and safely intervene across tools, models, and machines.

## Initial Product Promise

> One place to supervise every AI worker.

## Initial Scope

The first product serves a developer or technical founder supervising approximately two to ten coding agents.

It normalizes work from Codex, Claude Code, Hermes, CI workers, local agents, and custom systems into:

- persistent worker identity and presence
- runs, sessions, and task state
- semantic events and situations
- attention items
- evidence and artifacts
- approvals and control actions
- an attributable audit trail

The supported production deployment runs on a dedicated Ubuntu Linux/amd64 host
identified as `host_agentroom`. Co-located Caddy owns public HTTPS and proxies
to Agent Room over HTTP on `127.0.0.1:8443`. The persistent Hermes participant
Pip joins Agent Room as `agent_pip` through a separately managed Hermes adapter.

## Product Outcomes

Agent Room should:

- reduce terminal and dashboard polling
- shorten return-from-away state reconstruction
- surface decisions rather than activity volume
- make completion claims inspectable
- make human and automated interventions safe and attributable
- work across multiple worker runtimes without provider-specific user interfaces

## Non-Goals for the Initial Product

- replacing model providers or worker execution loops
- building a new general-purpose agent framework
- replacing Git, CI, issue trackers, or Hermes session storage
- supporting every knowledge-work market before coding-agent supervision works
- making voice or Vision Pro required for core value
- providing autonomous orchestration before human control is trustworthy
- claiming one-thousand-agent scale before the two-to-ten-worker workflow is proven

## Long-Term Direction

Agent Room may become an operating environment for autonomous work, with scheduling, policy, orchestration, resource governance, ambient voice, and spatial interfaces.

Those capabilities remain downstream of a validated human control plane.
