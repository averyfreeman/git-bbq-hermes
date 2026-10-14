# Git BBQ

Git BBQ is the Go-first scaffolding and lifecycle layer around Matt Pocock's
architecture skills. The division of responsibility is intentionally narrow:

- Matt skills own domain language, architecture reasoning, and the canonical ADR
  and context conventions.
- Git BBQ owns project bootstrap, language-specific agent skills, Git policy,
  deterministic projections, validation, and its Hermes-native adapter.
- Hermes remains the reasoning runtime; Git BBQ does not call another model or
  replace Matt's semantic interview.

## Project contract

`git-bbq init` writes operational state and the agent-facing router. A normal
project contains:

| Path | Role |
| --- | --- |
| `CONTEXT.md` | Matt-native project vocabulary and domain language |
| `CONTEXT-MAP.md` | Root context map for future context-specific vocabulary |
| `docs/adr/NNNN-slug.md` | The only architectural decision source of truth |
| `.githabits.yaml` | Granular policy for init, branch, stage, commit, tag, remote, and push |
| `.gitbbq-manifest.yaml` | Project problem, languages, and pinned Matt dependency |
| `AGENTS.md` | Thin project agent router |
| `.agents/skills/` | Githabits plus selected language skills |
| `.agents/mattpocock/DEPENDENCY.yaml` | Upstream Matt Pocock skills repository and commit pin |
| `architecture-contract.yaml` | Generated contract projection |
| `implementation-plan.md` | Generated implementation projection |
| `docs/adr/index.json` | Generated ADR retrieval projection |

The ADR directory is created lazily when the first ADR is recorded, matching the
Matt format. Generated projections can be recreated with `git-bbq project` and
are never a second decision history.

## Existing repositories

Use `git-bbq assess` first. It is read-only and reports proposed paths and
conflicts. `git-bbq apply --approve` is the separate persistence step; it
rechecks the repository before writing. Existing files are preserved unless
`--force` is explicitly supplied.

Every new scaffold records hashes in `.gitbbq/ownership.json`. Run
`git-bbq uninstall [path] --json` to review removable files; only
`git-bbq uninstall [path] --approve` removes unchanged files. Modified,
symlinked, reparse-point, and non-regular files or parent paths are reported as
conflicts and preserved. Unowned directories are preserved as well.

Supported language profiles are Go, Python, TypeScript, JavaScript, Rust, Java,
and C#. Explicit `--language` values take precedence over saved project
configuration and user preferences and bypass detection. Otherwise, interactive
`init` and `apply` show all detected candidates with evidence paths, use any
saved selection as the proposed set, and let the user confirm or edit the full
set. A final confirmation is required before files are written. Non-interactive
setup uses a saved project or user selection; if neither exists, it reports
candidate evidence and stops without writing. Detection considers supported
project markers and first-party source files in nested projects while skipping
common generated and vendored directories.

The generated `AGENTS.md`, focused language skills, and
`implementation-plan.md` projection include profile-specific guidance and
common command examples. They direct agents to prefer commands declared by the
repository and its CI. Git profile selection is also explicit: `manual` and
`guided` authorize no Git mutations by default, while `autonomous` enables the
individual action switches until a project changes them.

CLI flags take precedence over project configuration, and project configuration
takes precedence over the optional user-level preference file. Interactive
setup asks whether the selected profile and languages should be remembered. The
`--remember` flag makes that choice explicit in non-interactive use.

`git-bbq validate` checks any existing architecture-contract, implementation
plan, and ADR-index projections against their canonical inputs. It ignores the
ADR index's volatile generation timestamp; ADR index paths are repository-
relative so the projection survives repository moves. Validation reports
projection drift with a `git-bbq project` repair instruction. Projection files
remain optional until they are generated. Validation also scans bounded architecture and Git policy
artifacts for secret-like values; diagnostics identify only the file, line, and
finding type. `git-bbq project` validates canonical inputs and can regenerate
stale projections.

## Migrating AI Software Architect projects

Run `git-bbq migrate [path]` to review a read-only assessment of an older
`.adr-scaffold.yaml` and `.ai-architect/` project. The importer carries only
validated language/problem fields and selected decision fields into Git BBQ's
current manifest and Matt-native ADR format. It keeps legacy files in place,
does not import the old structured contract or handoff, and reports target
conflicts. Apply with `git-bbq migrate --approve [path]`. If the existing
`.githabits.yaml` is incompatible, review the assessment and add
`--archive-legacy-githabits` to preserve and replace it. See
[the conversion and conflict rules](./agents/legacy-migration.md).

The Git mutation seam has a read-only planning step and an explicit execution
step:

```sh
git-bbq githabits plan --action stage --path docs/adr/0001-use-matt-adrs.md --json
git-bbq githabits plan --action init --json
git-bbq githabits plan --action branch --branch feature/git-bbq --json
git-bbq githabits plan --action remote --json
git-bbq githabits plan --action remote --provision-remote --json
git-bbq githabits plan --action commit --message "feat: record architecture decision" --json
git-bbq githabits plan --action tag --tag v0.1.1 --existing-tag v0.1.0 --json
git-bbq githabits plan --action push --branch main --json
git-bbq githabits execute --approve --action init --json
git-bbq githabits execute --approve --action branch --branch feature/git-bbq --json
git-bbq githabits execute --approve --action remote --json
git-bbq githabits execute --approve --action remote --provision-remote --json
git-bbq githabits execute --approve --action commit --message "feat: record architecture decision" --json
git-bbq githabits execute --approve --action tag --tag v0.1.1 --existing-tag v0.1.0 --json
git-bbq githabits execute --approve --action push --branch main --json
```

Plans render argument-separated Git commands and report whether `.githabits.yaml`
authorizes the action. When `push.after_commit` or `push.after_tag` is enabled,
commit and tag plans include their required branch or tag push as an ordered
follow-up; planning fails closed until the push action and configured remote are
available. They never execute Git, and every plan still requires host-level
approval. `execute --approve` revalidates the plan, invokes Git without a shell,
applies a bounded timeout, and rejects tampered commands and force flags. A
failed follow-up push reports that the commit or tag already succeeded. Remote
planning reads `remote.alias` and `remote.url` from a pending
`.githabits.yaml` configuration and emits only `git remote add`. After that
approved command succeeds, the CLI persists `remote.status: configured`; a
failed command leaves the status pending. Push planning emits a command only
after the project remote is explicitly marked `configured`; staging rejects
absolute and parent-traversal paths, and conventional-commit profiles reject
messages without a recognized commit type. SemVer profiles require tags in the
`vMAJOR.MINOR.PATCH` form, with optional prerelease and build metadata.
Agents can pass repeated `--existing-tag` values from their read-only tag
inventory; planning rejects duplicate or older release tags.
The initial-baseline workflow is deliberately separate: it preserves all
published remote tags, removes only tags proven to be upstream-imported and
absent from `origin`, and creates the configured initial tag without rewriting
any remote ref. It fails if that exact initial tag already exists locally or on
the configured remote. Use `githabits release --since <initial-tag>` when the
new repository's release history begins at that baseline.
`--provision-remote` is a separate approval-gated GitHub CLI plan using
`gh repo create --source . --remote <alias>`; it never includes `--push`.

## Hermes integration

The plugin package is built from the same Go CLI:

```sh
python3 scripts/build_hermes_plugin.py \
  --output .tmp/git-bbq-hermes \
  --target x86_64-linux \
  --force
hermes plugins doctor --ci .tmp/git-bbq-hermes
hermes plugins validate --json .tmp/git-bbq-hermes
```

`plugins/git-bbq/plugin.yaml` declares the native plugin and its typed tool
surface. `__init__.py`, `tools.py`, and `schemas.py` register the shared slash
command, tools, and bundled skills; tool handlers delegate to the packaged Go
runtime. They do not reimplement Go business logic. The builder includes only
the selected upstream skills, their license, and the requested runtime target;
it does not create portable Agent Plugin manifests or lifecycle hook files.

Local filesystem and Git operations require a selected workspace and do not
auto-clone a repository. Remote GitHub Issues operations require an explicit
owner/repository target when no local checkout is selected. Mutating plugin
tools are protected by Hermes's host-level `pre_tool_call` approval gate; the
adapter passes the Go CLI's `--approve` flag only after approval. See
`docs/LOCAL_PLUGIN.md` for local build and installation guidance and
`docs/PLUGIN_SUBMISSION.md` for Hermes catalog preparation.

The first release targets the shared tools and skills that Hermes can expose
across its agent surfaces. Specialized Desktop panes and TUI widgets are
deferred; no host-specific UI integration is required to use the shared plugin.

Generate public contract schemas with:

```sh
go run ./cmd/git-bbq schema .
```

The schemas are written under `schemas/gitbbq/` and are derived from the Go
contracts rather than hand-maintained copies. Planner and executor responses are
available as `schemas/gitbbq/githabits-plan.schema.json` and
`schemas/gitbbq/githabits-execution.schema.json`.

An explicit `git-bbq update --approve --commit <pin>` changes the Matt dependency
pin in both the manifest and its dependency metadata. Validation never fetches or
silently changes the pinned dependency.

The plugin source is pinned directly to `mattpocock/skills`. The root
`git-bbq-curation.json` selects all stable skills in the upstream
`engineering` and `productivity` categories; package entries retain their
upstream names and source paths. The generated-project dependency metadata uses
the same upstream repository and commit while keeping the existing
`.agents/mattpocock` path and `CONTEXT.md` / `CONTEXT-MAP.md` project format.
That generated-project context contract is separate from the root `GLOSSARY.md`
used by the upstream setup workflow in this repository.

## External Matt dependency

New projects record the upstream repository and commit under
`.agents/mattpocock/DEPENDENCY.yaml`. The current pin is
`https://github.com/mattpocock/skills.git` at
`b0618bc436ad893b3c5e84e55fba86586d34a404`. The existing dependency path
contract remains `.agents/mattpocock`. Git BBQ does not silently fetch or
replace a project dependency during validation.
