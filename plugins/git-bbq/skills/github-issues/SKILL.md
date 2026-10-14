---
name: github-issues
description: Use Git-BBQ's typed GitHub issue tools for safe tracker operations.
---

# GitHub Issues

Use Git-BBQ's namespaced tools for tracker work. `git_bbq_issues_list` and `git_bbq_issue_get` are read-only. For remote-only operations, provide an explicit `owner` and `repo`; when a `workspace` is selected, its `origin` determines the repository. Never clone or infer a target checkout. The CLI delegates authentication to `gh api`, which uses the active GitHub CLI session or `GH_TOKEN` / `GITHUB_TOKEN` inherited from the active Hermes profile. Never request, paste, log, or reveal credentials.

## Create and organize

Use `git_bbq_issue_create` for specs and implementation tickets. If no label is supplied, Git-BBQ applies `needs-triage`; otherwise select labels from the repository's triage vocabulary (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). Mark implementation tickets `ready-for-agent` only when their acceptance criteria are clear. Keep the parent spec and child implementation issues in the same repository.

Use `git_bbq_issue_subissue` to create a native parent → child relationship. Use `git_bbq_issue_block` with the blocked issue in `issue` and its blocker in `by`; do not substitute prose-only links for dependency relationships.

## Edit and verify

Use `git_bbq_issue_edit`, `git_bbq_issue_labels`, `git_bbq_issue_assign`, `git_bbq_issue_comment`, `git_bbq_issue_close`, and `git_bbq_issue_reopen` for the corresponding operation. Hermes's pre-tool approval gate protects every write; do not pass a model-generated `approved` field. A `session` approval for the per-turn rule covers the planned writes in that workflow turn; a one-shot approval covers only one operation.

After each external write, read the exact issue or relationship back before reporting success. Treat API errors as failures, do not infer success from an attempted command, and avoid echoing raw API error output. Git-BBQ rejects credential-like issue content and withholds responses that look credential-bearing.
