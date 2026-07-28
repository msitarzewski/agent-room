# Third-Party Notices

Agent Room depends on third-party software. The dependency lockfiles and the
SPDX SBOM generated for each release are the authoritative version inventory.
The release archive includes this file, the Agent Room license and notice, and
the complete redistributed runtime license texts under `third_party/`.

## Runtime Dependencies

| Component | License |
| --- | --- |
| `github.com/coder/websocket` | ISC |
| `github.com/coreos/go-oidc/v3` | Apache-2.0 |
| `github.com/go-jose/go-jose/v4` | Apache-2.0 / BSD-3-Clause |
| `github.com/google/jsonschema-go` | MIT |
| `github.com/jackc/pgpassfile` | MIT |
| `github.com/jackc/pgservicefile` | MIT |
| `github.com/jackc/pgx/v5` | MIT |
| `github.com/jackc/puddle/v2` | MIT |
| `github.com/modelcontextprotocol/go-sdk` | Apache-2.0 / MIT transition |
| `github.com/segmentio/asm` | MIT |
| `github.com/segmentio/encoding` | MIT |
| `github.com/yosida95/uritemplate/v3` | BSD-3-Clause |
| `golang.org/x/crypto` | BSD-3-Clause |
| `golang.org/x/oauth2` | BSD-3-Clause |
| `golang.org/x/sync` | BSD-3-Clause |
| `golang.org/x/sys` | BSD-3-Clause |
| `golang.org/x/text` | BSD-3-Clause |
| `react` | MIT |
| `react-dom` | MIT |
| `scheduler` | MIT |

The Model Context Protocol Go SDK is transitioning from MIT to Apache-2.0.
Individual contributions remain under the applicable original license as
described in `third_party/go-licenses/github.com/modelcontextprotocol/go-sdk/LICENSE`.

## Required Attribution

CoreOS Project
Copyright 2014 CoreOS, Inc.

This product includes software developed at CoreOS, Inc.
(http://www.coreos.com/).

## Development Dependencies

Development and verification tools are not shipped as part of the Agent Room
runtime archive. Their licenses remain recorded in `web/package-lock.json`,
Go module metadata, container image metadata, and each tool's own distribution.
They include permissive MIT, Apache-2.0, ISC, BSD, BlueOak-1.0.0, CC0-1.0,
CC-BY-4.0, and MPL-2.0 terms.

This notice is informational and does not replace or modify any third-party
license.
