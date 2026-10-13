---
status: accepted
---

# Keep GitHabits as the canonical Git policy

`.githabits.yaml` is the single per-project source of truth for Git-related policy, and an optional user-level preset may seed new project files while allowing repository-specific overrides; other project instructions must not duplicate those policy values. The redesigned contract will represent authorization scope, commit/release semantics, tag baseline, push behavior, and remote-creation visibility policy as structured fields, while retaining compatibility with existing project configurations.