# Security Policy

## Reporting a Vulnerability

Please do not report suspected vulnerabilities in a public issue, discussion,
or pull request.

Use GitHub's private vulnerability reporting for this repository:

https://github.com/msitarzewski/agent-room/security/advisories/new

Include the affected version or source revision, deployment context,
reproduction steps, expected and observed behavior, and the security impact.
Remove credentials, tokens, private addresses, customer data, and other
sensitive material from the report.

The maintainer will acknowledge the report, validate its scope, coordinate a
fix and disclosure where warranted, and credit reporters who want attribution.
Response and remediation timing depends on severity, reproducibility, and
maintainer availability; this project does not promise a fixed service-level
agreement.

## Supported Versions

Until the first tagged release, security fixes target the latest revision of
the default branch. After tagged releases begin, the supported release range
will be documented here.

## Security Boundaries

Agent Room is a control plane, so reports involving authorization, project
isolation, approval integrity, secret exposure, artifact integrity, adapter or
MCP scope, and public/private listener separation are especially important.

The repository contains documented local-only credentials and reserved example
addresses. They are test fixtures, not production secrets. Never reuse them in
a deployed environment.

Only test systems you own or are explicitly authorized to assess.
