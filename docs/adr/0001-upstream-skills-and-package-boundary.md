# Use upstream Matt Pocock skills as the plugin source

**Status:** Superseded by [ADR 0003](./0003-hermes-native-plugin-boundary.md)

## Context

Git BBQ currently adapts and renames upstream Matt Pocock skills in a
maintained derivative repository. That duplicates content maintenance and
breaks the established skill names people use across projects. The plugin
builder also needs a package layout that discovers each skill, while the
upstream repository groups skills by category. Provider invocation metadata
does not use a field allowed by the Agent Skills specification.

Git BBQ-generated projects already have a separate `CONTEXT.md` and
`CONTEXT-MAP.md` contract. The upstream setup workflow uses `GLOSSARY.md` and
`GLOSSARY-MAP.md`. Running setup in the Git BBQ source repository should not
silently change the generated-project format.

## Decision

Use `mattpocock/skills` at an explicit Git commit as the plugin's only upstream
skill source. Keep the source submodule pointed at that repository. Keep the
curation manifest, selection policy, package builder, and package validation in
Git BBQ.

Select every stable skill in the upstream `engineering` and `productivity`
categories. Export selected directories from the pinned Git tree and place them
at immediate `skills/<upstream-name>/` paths so Agent Plugins discovers them.
Keep the package directory and frontmatter name equal to the original
upstream directory name. Do not publish legacy aliases or rewrite upstream
skill instructions and auxiliary files.

The builder may adapt non-standard fields from the packaged frontmatter because
those fields are outside the Agent Skills schema. It must first verify that
`disable-model-invocation: true` is covered by the upstream `agents/openai.yaml`
Codex `allow_implicit_invocation: false` policy. Preserve `argument-hint` as a
namespaced value inside standard `metadata`. This narrow metadata adapter does
not modify the submodule or upstream skill body.

Use a root `GLOSSARY.md`, root `docs/adr/`, and `docs/agents/` integration docs
for this repository. Keep generated projects on the current `CONTEXT.md` and
`CONTEXT-MAP.md` format. Point their dependency metadata at the same upstream
repository and commit without changing the existing `.agents/mattpocock` path.

Keep lifecycle hooks for local Codex installations. The package is not a
candidate for OpenAI's public plugin directory while hooks remain enabled.
Keep Effective HTML as an optional companion plugin rather than another Git
BBQ package dependency.

Declare the bundled upstream `setup-matt-pocock-skills` skill as OpenAI plugin
onboarding in both the portable and Codex compatibility manifests. Validate
that both paths agree and resolve to the packaged setup skill. This uses the
plugin's existing setup capability as the first-run entry point without
modifying upstream skill content.

## Consequences

- New upstream skills in the selected categories require an explicit pin and
  curation update; the builder checks that the manifest covers the full stable
  selection at that commit.
- Public skill names track upstream and may change when Git BBQ adopts a newer
  commit. The source commit remains visible in package provenance.
- Packaging uses an immediate-child projection because plugin discovery does
  not recursively find skills below category directories.
- The root glossary describes Git BBQ's packaging domain. Generated project
  context remains a separate contract.
- Local Codex distribution retains its lifecycle hooks; public directory
  publication remains unavailable under current OpenAI guidance.
- New plugin installations have a manifest-declared setup entry point, while
  users can still invoke the setup skill directly when needed.
- Effective HTML can assist with visual direction, wireframes, prototypes,
  architecture diagrams, and roadmap artifacts, but does not replace code
  changes, architecture records, or testable implementation.
