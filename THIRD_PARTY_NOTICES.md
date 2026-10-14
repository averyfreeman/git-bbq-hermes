<!--
SPDX-FileCopyrightText: 2026 Git-BBQ contributors
SPDX-License-Identifier: MIT
-->

# Third-Party Notices

## Matt Pocock skills

The plugin package includes selected skills from
[Matt Pocock's skills repository](https://github.com/mattpocock/skills), pinned
at commit `b0618bc436ad893b3c5e84e55fba86586d34a404`. The upstream MIT license is
included in built packages at `licenses/mattpocock-skills/LICENSE`. Skill text
and support files remain upstream content.

## Go module dependency graph

The bundled Go executable is built from the module graph recorded by `go.mod`
and `go.sum`. The graph at this revision is:

| Module | Version |
| --- | --- |
| `cloud.google.com/go/compute/metadata` | `v0.3.0` |
| `github.com/golang-jwt/jwt/v5` | `v5.3.1` |
| `github.com/google/go-cmp` | `v0.7.0` |
| `github.com/google/jsonschema-go` | `v0.4.3` |
| `github.com/modelcontextprotocol/go-sdk` | `v1.8.0` |
| `github.com/segmentio/asm` | `v1.1.3` |
| `github.com/segmentio/encoding` | `v0.5.4` |
| `github.com/yosida95/uritemplate/v3` | `v3.0.2` |
| `golang.org/x/oauth2` | `v0.35.0` |
| `golang.org/x/sync` | `v0.20.0` |
| `golang.org/x/sys` | `v0.41.0` |
| `golang.org/x/time` | `v0.15.0` |
| `golang.org/x/tools` | `v0.42.0` |
| `gopkg.in/check.v1` | `v0.0.0-20161208181325-20d25e280405` |
| `gopkg.in/yaml.v3` | `v3.0.1` |

Consult each module's upstream repository for its license and notices. Exact
source checksums are recorded in `go.sum`; the module list above describes the
resolved graph and does not imply that every module is linked into every target
binary.
