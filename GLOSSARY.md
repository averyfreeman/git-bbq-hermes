# Git BBQ glossary

Use these terms consistently in this repository's issue titles, specs, ADRs,
and implementation discussions.

| Term | Meaning |
| --- | --- |
| **Upstream skill source** | The pinned `mattpocock/skills` Git repository recorded in the root curation manifest and `.agents/skills` submodule. |
| **Curation manifest** | Git BBQ-owned metadata selecting which upstream skill directories enter the plugin package. It does not rewrite their names or instructions. |
| **Skill projection** | A selected upstream skill directory copied into the plugin's immediate `skills/<name>/` discovery path. |
| **Go core** | Git BBQ's Go CLI and `internal` packages, which remain the canonical project, ADR, validation, and Git-policy engine. |
| **Hermes-native plugin** | A Python plugin declared by `plugin.yaml` and loaded through `register(ctx)`; it exposes tools and bundled skills through Hermes. |
| **Hermes host adapter** | The plugin layer that maps typed Hermes tool calls to the bundled Go core without duplicating project or GitHub Issue behavior. |
| **Remote-only issue operation** | A GitHub Issues action against configured `owner/repo` that does not require a local checkout. |
| **Codex compatibility manifest** | Historical package metadata documented by ADRs 0001–0002; it is retired from the Hermes-only distribution. |
| **Generated project context** | Git BBQ's existing `CONTEXT.md` and `CONTEXT-MAP.md` files created for scaffolded repositories. This format is separate from this repository's root glossary. |
| **Language candidate** | A supported language proposed by read-only repository detection, together with the paths that support it. It is not selected project configuration until the user confirms it or an existing explicit selection applies. |
| **Language evidence** | A recognized project marker or first-party source file used to propose a language candidate. Evidence is shown so users can assess ambiguous and polyglot repositories. |
| **Language profile** | Git BBQ's language-specific operating guidance used by the generated agent router, focused skill, and implementation-plan projection. |
| **Generated projection** | A deterministic artifact derived from the manifest and Matt-native project records. It supports retrieval and validation but is not an independent source of truth. |
| **Legacy project migration** | An explicit, additive conversion of compatible AI Software Architect project metadata and decisions into Git BBQ's current project format while preserving the original files. |

Do not use “renamed skill” for a packaged upstream skill. Its package directory
and frontmatter name match the upstream directory name. Do not call the Git
BBQ-owned curation manifest upstream content.
