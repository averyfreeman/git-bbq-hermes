---
status: accepted
---

# Separate workspace operations from remote issue operations

Local file and Git operations are scoped to the explicitly selected checkout and never create a clone implicitly; when present, its `origin` identifies the repository. Remote GitHub Issue operations may run without a checkout, but require an explicit configured `owner/repo` and authentication from `gh` or the active Hermes profile's `GH_TOKEN`/`GITHUB_TOKEN`; credentials are never requested in chat. This boundary allows the same plugin tools to work across Hermes surfaces without silently choosing a workspace or repository.