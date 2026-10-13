---
status: accepted
---

# Make Hermes the only host adapter and preserve the Go core

Git-BBQ Hermes is a Hermes-only distribution: retain the Go CLI as the canonical project and issue engine, replace Codex manifests and hooks with a native Hermes Python plugin (`plugin.yaml` and `register(ctx)`), and expose typed tools plus bundled pinned skills through Hermes' shared tool registry. Native handlers invoke the bundled Go runtime rather than duplicating domain or GitHub Issue behavior. The prior Codex distribution decisions in ADRs 0001–0002 are superseded; the upstream skill source and curation boundary remain. Surface-specific Desktop and TUI UI extensions are deferred because the shared tools and skills cover the first-release surfaces.