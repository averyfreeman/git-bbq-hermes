---
status: superseded
---

# Selectively consolidate AI Software Architect capabilities into Git BBQ

> Superseded by [ADR 0003](./0003-hermes-native-plugin-boundary.md) for the
> distribution and host-adapter boundary. The project scaffolding and migration
> decisions below remain part of Git-BBQ's architecture history.

## Context

AI Software Architect at `c7045bd` contains a language detector, language
profiles, a structured architecture-contract and ADR system, deterministic
bundle validation, and a migration path for older `.ai-architect` projects.
Git BBQ `v0.4.0` already supports multiple selected language profiles, keeps
Matt-native context and ADRs as project records, validates its project contract,
and generates architecture, implementation-plan, and ADR-index projections.

The older detector chooses one language using marker priority and top-level
files. The older bundle validator expects AI Software Architect's structured
decision schema and accepted-decision references. Neither behavior can be
copied unchanged into Git BBQ: repositories can be polyglot, and generated Git
BBQ projections derive from different canonical inputs.

## Decision

Adopt the useful mechanics within Git BBQ's existing Go interfaces:

- Detect all supported language candidates read-only and return evidence paths.
  Interactive setup lets the user edit and confirm the full set before writing.
  Existing CLI, project, and saved preference selections retain precedence;
  non-interactive setup without a stored selection or `--language` reports
  candidates and stops.
- Enrich the existing `AGENTS.md`, language skills, and
  `implementation-plan.md` with the selected language profiles. Preserve the
  generated `CONTEXT.md` and `CONTEXT-MAP.md` contract.
- Extend project validation to check Git BBQ's own generated projections and
  scan architecture artifacts for secret-like values without returning those
  values in diagnostics.
- Add a read-only legacy migration preview and an explicit approval-gated apply
  path. Convert only validated compatible data, preserve legacy sources, and
  report target conflicts.
- Build a local plugin variant with the five lifecycle hooks and a
  public-oriented, hook-free variant with the same curated skills and local Go
  CLI runtime. The latter does not provide access to local files from web hosts.

Do not import AI Software Architect's structured contract/ADR schema or full
runtime. Defer MCP and heuristic source dependency-graph analysis.

## Rationale

Keeping one canonical project record set prevents conflicting architecture
histories. Evidence-backed multi-language detection improves on a single
primary-language guess while requiring a user decision before a fresh detected
set can affect files. Reusing Git BBQ's validator, projections, ownership
ledger, and approval seams retains its existing safety properties. A narrow
legacy migration preserves prior project work without keeping two active
formats.

## Consequences

- Detection and migration are additive to existing CLI behavior; stored
  selections remain usable in non-interactive workflows.
- Projection validation must be read-only and compare semantic content rather
  than volatile generation timestamps. ADR index paths are repository-relative
  so the projection remains portable when a repository moves.
- Migration must preserve original files and require explicit approval before
  archiving an incompatible legacy `.githabits.yaml`.
- Both plugin variants share skill and CLI sources. The local variant retains
  hooks; the public-oriented variant excludes hook files and manifest
  references. Public submission remains a separate release action.
- `GLOSSARY.md` defines language candidates, evidence, profiles, projections,
  and migration for consistent implementation language.
