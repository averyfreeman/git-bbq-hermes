"""Native Hermes adapter for the canonical Git-BBQ Go CLI."""

from __future__ import annotations

import logging
from pathlib import Path
import shlex

from . import schemas, tools

logger = logging.getLogger(__name__)
_PLUGIN_DIR = Path(__file__).resolve().parent
_MUTATING_TOOLS = frozenset({
    "git_bbq_init",
    "git_bbq_project",
    "git_bbq_adr_new",
    "git_bbq_adr_index",
    "git_bbq_migrate",
    "git_bbq_git_execute",
    "git_bbq_baseline_execute",
    "git_bbq_issue_create",
    "git_bbq_issue_edit",
    "git_bbq_issue_labels",
    "git_bbq_issue_assign",
    "git_bbq_issue_comment",
    "git_bbq_issue_close",
    "git_bbq_issue_reopen",
    "git_bbq_issue_subissue",
    "git_bbq_issue_block",
})


def _command(raw_args: str) -> str:
    try:
        parts = shlex.split(raw_args)
    except ValueError as exc:
        return tools.error_result("invalid_arguments", str(exc))
    if not parts or parts[0] not in {"project", "validate"} or len(parts) > 2:
        return tools.error_result("invalid_arguments", "Usage: /git-bbq <project|validate> [workspace]")
    arguments: dict[str, object] = {"workspace": parts[1]} if len(parts) == 2 else {}
    if parts[0] == "project":
        return tools.git_bbq_project(arguments)
    return tools.git_bbq_validate(arguments)


def _on_pre_tool_call(
    tool_name: str = "", args: dict | None = None, session_id: str = "", turn_id: str = "", **_: object
) -> dict | None:
    """Require Hermes's human approval gate before every mutating tool call."""
    if tool_name not in _MUTATING_TOOLS:
        return None
    arguments = args if isinstance(args, dict) else {}
    if tool_name == "git_bbq_migrate" and not (
        arguments.get("apply") is True or arguments.get("archive_legacy_githabits") is True
    ):
        return None
    if not turn_id:
        return {"action": "block", "message": "Git-BBQ mutations require an identifiable interactive workflow turn."}
    return {
        "action": "approve",
        "message": f"Git-BBQ mutation via {tool_name}; approve the planned writes for this workflow turn.",
        "rule_key": f"git-bbq-workflow:{session_id}:{turn_id}",
    }


def register(ctx) -> None:
    """Register typed Go-backed tools, approval hook, command, and bundled skills."""
    ctx.register_hook("pre_tool_call", _on_pre_tool_call)
    for name, schema in schemas.TOOLS.items():
        ctx.register_tool(
            name=name,
            toolset="git_bbq",
            schema=schema,
            handler=getattr(tools, name),
            emoji="🧭",
        )
    ctx.register_command(
        "git-bbq",
        _command,
        description="Inspect or regenerate Git-BBQ project state with /git-bbq <validate|project> [workspace]",
        args_hint="<validate|project> [workspace]",
    )
    skills_dir = _PLUGIN_DIR / "skills"
    for child in sorted(skills_dir.iterdir()) if skills_dir.is_dir() else []:
        skill_md = child / "SKILL.md"
        if child.is_dir() and skill_md.is_file():
            try:
                ctx.register_skill(child.name, skill_md)
            except Exception:
                logger.warning("Failed to register Git-BBQ skill %s", child.name, exc_info=True)
