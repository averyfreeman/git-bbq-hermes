# Product spec: upstream skills and Git BBQ plugin packaging

> Status: this original Codex/portable-plugin proposal is superseded by
> [ADR 0003](../adr/0003-hermes-native-plugin-boundary.md). It is retained as
> historical decision context; use the Hermes integration and release guides
> for current behavior.

## Problem Statement

Git BBQ has been maintaining a vocabulary-adapted derivative of Matt Pocock's
skills. The derivative creates an ongoing content-maintenance burden, while
renamed skills do not match the names used by upstream users. The plugin also
needs repository setup, triage, research, architecture, specification, and
ticket workflows that depend on infrastructure the adaptation did not carry
forward.

The package needs to preserve upstream skill provenance while satisfying
standard skill discovery and plugin metadata rules. Git BBQ's generated
projects already use `CONTEXT.md` and `CONTEXT-MAP.md`, while the upstream setup
workflow uses a glossary. The integration must give the Git BBQ repository its
own glossary without changing the generated-project format.

## Solution

Use the upstream Matt Pocock skills repository at an explicit Git commit as the
plugin's source. Keep the submodule pointed directly at upstream, and keep
selection metadata and packaging code in Git BBQ. Package all stable upstream
engineering and productivity skills under their upstream names in immediate
plugin skill directories. Do not add legacy aliases or rewrite upstream
instructions.

Keep the portable Agent Plugins manifest and Codex compatibility manifest.
Validate skill names, metadata, discovery paths, provenance, and OpenAI plugin
metadata constraints. Preserve the semantics of upstream manual-only skills
through the adjacent Codex invocation policy while adapting unsupported
frontmatter into Agent Skills-compatible metadata. Include the upstream license
in the package. Declare the bundled upstream setup skill as the OpenAI plugin
onboarding skill in both manifests, and ensure the declared path resolves in
the built package.

Set up this repository for the upstream workflows with GitHub Issues, default
triage labels, PR triage disabled, root `GLOSSARY.md`, and ADR documentation.
Keep generated Git BBQ projects on their existing context format. Evaluate
Effective HTML as an optional companion for visual product direction,
interaction prototypes, diagrams, and roadmaps; do not vendor it into Git BBQ.
Keep lifecycle hooks for local Codex use and defer public plugin-directory
submission because packages containing hooks are currently ineligible.

## User Stories

1. As a Git BBQ user, I want familiar upstream skill names, so that I can use
   the same skill calls across repositories and tools.
2. As a maintainer, I want an explicit upstream source commit, so that each
   package can be traced to reproducible skill content.
3. As a maintainer, I want Git BBQ to own its curation policy, so that the
   plugin can select a stable subset without maintaining a fork of skill text.
4. As a maintainer, I want the builder to detect missing or stale stable skill
   selections, so that an upstream update cannot silently omit skills.
5. As a plugin user, I want each skill discovered in the plugin's root skill
   directory, so that Codex can load it by its upstream name.
6. As a skill author or user, I want upstream instructions and support files
   preserved, so that plugin packaging does not create a diverging workflow.
7. As a user, I want manual-only skills to remain manual-only after packaging,
   so that the Agent Skills metadata adapter does not change invocation
   behavior.
8. As a maintainer, I want both portable and Codex-specific plugin manifests,
   so that the package keeps its cross-client identity and current Codex
   compatibility.
9. As a maintainer, I want package validation to enforce skill layout, names,
   frontmatter limits, provenance, and plugin metadata limits, so that invalid
   packages fail before installation.
10. As a user, I want the upstream license distributed with bundled skills, so
    that attribution remains available in the installed package.
11. As a maintainer, I want GitHub issue triage configured with the default
    labels and no PR triage, so that the upstream triage skill can use the
    repository's real tracker without changing existing labels.
12. As a maintainer, I want the repository to have a root glossary and ADR
    guidance, so that upstream architecture skills use one vocabulary and one
    decision history in this codebase.
13. As a user of generated Git BBQ projects, I want their existing context
    files and paths preserved, so that adopting the plugin source does not
    invalidate established project contracts.
14. As a product designer or engineer, I want clear guidance on when to use
    Effective HTML, so that I can explore user flows visually without making
    it a required Git BBQ dependency.
15. As a local Codex user, I want Git BBQ lifecycle hooks retained and their
    trust requirements documented, so that local installations continue to
    provide the existing repository safeguards.
16. As a community contributor, I want the product spec and implementation
    tickets linked on GitHub with native dependencies, so that I can see the
    work, blockers, and review state.
17. As a maintainer, I want the ticket workflow options documented, so that
    future projects can choose GitHub Issues, GitLab Issues, local Markdown, or
    another tracker through its guide.
18. As a new plugin user, I want the upstream setup skill offered as plugin
    onboarding, so that I can configure Git BBQ's repository workflows after
    installation.

## Implementation Decisions

- Upstream Git content is the only source for bundled Matt Pocock skills.
- Curation policy and package adaptation belong to Git BBQ.
- A package preserves upstream skill names, instructions, and auxiliary files.
- The builder reads only the pinned Git tree and records source provenance.
- A narrow metadata adapter preserves manual-only invocation through Codex
  policy and carries upstream argument hints into namespaced standard metadata.
- The package keeps a portable manifest and a Codex compatibility manifest.
- Both manifests declare `./skills/setup-matt-pocock-skills/SKILL.md` as the
  OpenAI onboarding skill; package validation checks the declarations and file.
- Git BBQ's root setup uses `GLOSSARY.md` and `docs/adr/`; generated projects
  retain `CONTEXT.md` and `CONTEXT-MAP.md`.
- Effective HTML remains a separate optional companion plugin.
- Lifecycle hooks stay in local packages; public plugin-directory submission
  is out of scope while those hooks are present.
- Recommend Effective HTML's `design-artifact` for visual product direction,
  `html-wireframe` and `html-prototype` for interaction exploration, and
  `html-diagram` and `html-plan` for architecture and roadmap artifacts. Keep
  it separately installed and use HTML artifacts when they improve review of
  a decision; code, tests, and ADRs remain the durable implementation record.
- GitHub Issues is the selected backend for this repository. GitLab Issues,
  local Markdown tickets, and other trackers remain supported workflow options
  when a project has the corresponding tracker guide.

## Testing Decisions

- Test curation parsing, complete stable-skill selection, source pin handling,
  and path safety using observable build behavior.
- Build a fresh package from the pinned upstream Git tree and validate every
  projected skill using the Agent Skills reference validator.
- Validate both plugin manifests, direct skill discovery paths, frontmatter
  names, the onboarding-skill declaration and packaged path, hook declarations,
  runtime launchers, license presence, and OpenAI metadata limits.
- Install the built package in an isolated Codex home and smoke-use setup,
  triage, research, architecture review, specification, and ticket workflows
  against a disposable fixture repository.
- Verify on GitHub that the spec is a parent issue, children are linked as
  sub-issues, `ready-for-agent` is set, and dependency edges match the intended
  order.

## Out of Scope

- Rewriting upstream skill prose or maintaining Git-BBQ-specific aliases.
- Bundling Effective HTML or modifying its source repository.
- Changing generated projects from `CONTEXT.md` to `GLOSSARY.md`.
- Removing local lifecycle hooks or submitting this package to the public
  plugin directory.
- Replacing GitHub Issues as this repository's configured tracker.

## Further Notes

The `to-tickets` workflow supports GitHub Issues, GitLab Issues, local Markdown
files under `.scratch/<feature>/issues/`, and other trackers such as Jira or
Linear when a project supplies the tracker guide. Use tracer-bullet tickets by
default; use expand–migrate–contract for broad mechanical refactors, and record
each dependency explicitly.
