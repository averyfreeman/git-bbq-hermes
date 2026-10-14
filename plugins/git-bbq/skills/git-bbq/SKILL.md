---
name: git-bbq
description: Use Git-BBQ's Hermes tools to initialize, inspect, validate, and maintain project architecture context.
---

# Git-BBQ on Hermes

Use the namespaced skills `git-bbq:<skill-name>` to load bundled workflows. The typed Git-BBQ tools call the canonical Go CLI and return `{"ok":true,"data":...,"error":null}` on success or a stable error envelope. Do not invoke the Go runtime through a shell.

## Project inspection

Use `git_bbq_assess` to preview scaffold changes and `git_bbq_validate` for read-only validation. Both use the current workspace unless the user explicitly selects another existing directory. Relative workspace paths cannot escape the current directory; never clone or infer a different checkout.

`git_bbq_project` regenerates architecture projections and writes files; `git_bbq_adr_index` regenerates its index. `git_bbq_init` initializes project records, `git_bbq_adr_new` creates a decision record, and `git_bbq_migrate` previews by default or applies when `apply: true`. Hermes's pre-tool approval gate protects mutating tool calls; never supply a model-generated approval flag. Keep project decisions in the repository's `docs/adr/` records and canonical context files; do not edit generated projections directly.

Use `git_bbq_githabits` to read `.githabits.yaml`, `git_bbq_git_plan` before a Git operation, and `git_bbq_git_execute` only within the user's authorized workflow. `git_bbq_release_plan` is read-only. `git_bbq_baseline_plan` previews inherited local-tag cleanup; `git_bbq_baseline_execute` requires authorization and must never rewrite tags already published to `origin` or tags in `upstream`.

The shared `/git-bbq validate [workspace]` command is read-only. `/git-bbq project [workspace]` is an explicit user request to regenerate projections. All tool handlers use fixed argument arrays with shell execution disabled.
