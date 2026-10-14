"""Typed handlers that invoke the bundled Git-BBQ Go executable."""

from __future__ import annotations

import json
import os
import platform
from pathlib import Path
import subprocess


CLI_TIMEOUT_SECONDS = 60


def _runtime_target() -> str:
    system = platform.system().lower()
    machine = platform.machine().lower()
    if system == "windows" and machine in {"amd64", "x86_64"}:
        return "windows-x86_64"
    if system == "darwin" and machine in {"arm64", "aarch64"}:
        return "aarch64-darwin"
    if system == "darwin" and machine in {"amd64", "x86_64"}:
        return "x86_64-darwin"
    if system == "linux" and machine in {"amd64", "x86_64"}:
        return "x86_64-linux"
    raise RuntimeError(f"Git-BBQ has no bundled runtime for {system}/{machine}")


def _resolve_cli_binary() -> Path:
    target = _runtime_target()
    executable = "git-bbq.exe" if target == "windows-x86_64" else "git-bbq"
    binary = Path(__file__).resolve().parent / "runtime" / "bin" / target / executable
    if not binary.is_file():
        raise RuntimeError(f"bundled Git-BBQ runtime is missing for {target}")
    return binary


def _resolve_workspace(value: object = None) -> Path:
    current = Path.cwd().resolve(strict=True)
    if value is None or not str(value).strip():
        workspace = current
    else:
        requested = Path(str(value).strip()).expanduser()
        is_relative = not requested.is_absolute()
        if is_relative:
            requested = current / requested
        workspace = requested.resolve(strict=True)
        if is_relative:
            try:
                workspace.relative_to(current)
            except ValueError as exc:
                raise ValueError("relative workspace must remain inside the current workspace") from exc
    if not workspace.is_dir():
        raise ValueError("workspace must be an existing directory")
    return workspace


def _profile_environment() -> dict[str, str]:
    environment = os.environ.copy()
    if "GIT_BBQ_CONFIG" not in environment:
        hermes_home = environment.get("HERMES_HOME")
        if hermes_home:
            environment["GIT_BBQ_CONFIG"] = str(Path(hermes_home) / "git-bbq" / "config.yaml")
    return environment


def error_result(code: str, message: str) -> str:
    return json.dumps({"ok": False, "data": None, "error": {"code": code, "message": message}})


def _success_result(data: object) -> str:
    return json.dumps({"ok": True, "data": data, "error": None})


def _invoke(command: list[str], workspace_value: object = None) -> str:
    try:
        workspace = _resolve_workspace(workspace_value)
        binary = _resolve_cli_binary()
    except (OSError, RuntimeError, ValueError) as exc:
        return error_result("workspace_or_runtime_error", str(exc))

    try:
        result = subprocess.run(
            [str(binary), *command],
            cwd=str(workspace),
            shell=False,
            capture_output=True,
            text=True,
            timeout=CLI_TIMEOUT_SECONDS,
            env=_profile_environment(),
            check=False,
        )
    except subprocess.TimeoutExpired:
        return error_result("timeout", "Git-BBQ CLI command exceeded its time limit")
    except ValueError:
        return error_result("invalid_arguments", "Git-BBQ CLI arguments contain an invalid value")
    except OSError:
        return error_result("runtime_error", "could not start the bundled Git-BBQ CLI")

    if result.returncode != 0:
        return error_result("cli_error", f"Git-BBQ CLI command failed (exit code {result.returncode})")
    try:
        data = json.loads(result.stdout)
    except (json.JSONDecodeError, TypeError):
        return error_result("invalid_cli_output", "Git-BBQ CLI returned invalid JSON")
    return _success_result(data)


def git_bbq_project(args: dict, **kwargs) -> str:
    """Regenerate the architecture projection after the pre-tool approval gate."""
    del kwargs
    return _invoke(["project", "--json"], args.get("workspace"))


def _required_string(args: dict, name: str) -> str:
    value = args.get(name)
    if not isinstance(value, str) or not value.strip() or "\x00" in value:
        raise ValueError(f"{name} must be a non-empty string")
    return value


def _optional_string(args: dict, name: str) -> str | None:
    value = args.get(name)
    if value is None or value == "":
        return None
    if not isinstance(value, str) or "\x00" in value:
        raise ValueError(f"{name} must be a string")
    return value


def _string_list(args: dict, name: str) -> list[str]:
    value = args.get(name, [])
    if not isinstance(value, list) or any(not isinstance(item, str) or "\x00" in item for item in value):
        raise ValueError(f"{name} must be a list of strings")
    return value


def _json_call(command: list[str], args: dict) -> str:
    try:
        return _invoke(command, args.get("workspace"))
    except ValueError as exc:
        return error_result("invalid_arguments", str(exc))


def git_bbq_assess(args: dict, **kwargs) -> str:
    del kwargs
    return _json_call(["assess", "--json"], args)


def git_bbq_init(args: dict, **kwargs) -> str:
    del kwargs
    try:
        command = [
            "init", "--json", "--here", "--name", _required_string(args, "project_name"),
            "--problem", _required_string(args, "problem"),
        ]
        languages = _string_list(args, "languages")
        if not languages:
            raise ValueError("languages must include at least one supported language")
        for language in languages:
            command.extend(["--language", language])
    except ValueError as exc:
        return error_result("invalid_arguments", str(exc))
    return _json_call(command, args)


def git_bbq_validate(args: dict, **kwargs) -> str:
    del kwargs
    return _json_call(["validate", "--json"], args)


def git_bbq_adr_new(args: dict, **kwargs) -> str:
    del kwargs
    try:
        command = ["adr", "new"]
        for name, flag in (("title", "--title"), ("context", "--context"), ("decision", "--decision"), ("why", "--why")):
            command.extend([flag, _required_string(args, name)])
        status = _optional_string(args, "status")
        if status:
            command.extend(["--status", status])
        command.append("--json")
    except ValueError as exc:
        return error_result("invalid_arguments", str(exc))
    return _json_call(command, args)


def git_bbq_adr_index(args: dict, **kwargs) -> str:
    del kwargs
    return _json_call(["adr", "index"], args)


def git_bbq_migrate(args: dict, **kwargs) -> str:
    del kwargs
    archive = args.get("archive_legacy_githabits") is True
    apply = args.get("apply") is True or archive
    if apply:
        command = ["migrate", "--json", "--approve"]
        if archive:
            command.append("--archive-legacy-githabits")
    else:
        command = ["migrate", "--json"]
    return _json_call(command, args)


def git_bbq_githabits(args: dict, **kwargs) -> str:
    del kwargs
    return _json_call(["githabits", "--json"], args)


def _git_action_command(args: dict, execute: bool) -> list[str]:
    command = ["githabits", "execute" if execute else "plan", "--action", _required_string(args, "action"), "--json"]
    if execute:
        command.append("--approve")
    if args.get("provision_remote") is True:
        command.append("--provision-remote")
    for name, flag in (("branch", "--branch"), ("message", "--message"), ("tag", "--tag")):
        value = _optional_string(args, name)
        if value is not None:
            command.extend([flag, value])
    for name, flag in (("paths", "--path"), ("existing_tags", "--existing-tag")):
        for value in _string_list(args, name):
            command.extend([flag, value])
    return command


def git_bbq_git_plan(args: dict, **kwargs) -> str:
    del kwargs
    try:
        command = _git_action_command(args, execute=False)
    except ValueError as exc:
        return error_result("invalid_arguments", str(exc))
    return _json_call(command, args)


def git_bbq_git_execute(args: dict, **kwargs) -> str:
    del kwargs
    try:
        command = _git_action_command(args, execute=True)
    except ValueError as exc:
        return error_result("invalid_arguments", str(exc))
    return _json_call(command, args)


def git_bbq_release_plan(args: dict, **kwargs) -> str:
    del kwargs
    command = ["githabits", "release", "--json"]
    try:
        since = _optional_string(args, "since")
    except ValueError as exc:
        return error_result("invalid_arguments", str(exc))
    if since is not None:
        command.extend(["--since", since])
    return _json_call(command, args)


def git_bbq_baseline_plan(args: dict, **kwargs) -> str:
    del kwargs
    return _json_call(["githabits", "baseline", "plan", "--json"], args)


def git_bbq_baseline_execute(args: dict, **kwargs) -> str:
    del kwargs
    return _json_call(["githabits", "baseline", "execute", "--approve", "--json"], args)


def _issue_target_command(args: dict, command: list[str]) -> str:
    try:
        workspace_value = _optional_string(args, "workspace")
        if workspace_value is not None:
            workspace_value = str(_resolve_workspace(workspace_value))
        owner = _optional_string(args, "owner")
        repository = _optional_string(args, "repo")
        if owner is not None:
            command.extend(["--owner", owner])
        if repository is not None:
            command.extend(["--repo", repository])
    except ValueError as exc:
        return error_result("invalid_arguments", str(exc))
    if workspace_value is not None:
        command.extend(["--workspace", workspace_value])
    return _invoke(command, workspace_value)


def _issue_number(args: dict, name: str = "number") -> int:
    value = args.get(name)
    if type(value) is not int or value < 1:
        raise ValueError(f"{name} must be a positive integer")
    return value


def _optional_issue_text(args: dict, name: str, allow_empty: bool = True) -> str | None:
    if name not in args or args[name] is None:
        return None
    value = args[name]
    if not isinstance(value, str) or "\x00" in value or (not allow_empty and not value.strip()):
        raise ValueError(f"{name} must be a valid string")
    return value


def git_bbq_issues_list(args: dict, **kwargs) -> str:
    del kwargs
    command = ["issues", "list"]
    state = args.get("state", "open")
    if state not in {"open", "closed", "all"}:
        return error_result("invalid_arguments", "state must be open, closed, or all")
    command.extend(["--state", state])
    return _issue_target_command(args, command)


def git_bbq_issue_get(args: dict, **kwargs) -> str:
    del kwargs
    try:
        number = _issue_number(args)
    except ValueError as exc:
        return error_result("invalid_arguments", str(exc))
    return _issue_target_command(args, ["issues", "get", "--number", str(number)])


def git_bbq_issue_create(args: dict, **kwargs) -> str:
    del kwargs
    try:
        title = _required_string(args, "title")
        body = _optional_issue_text(args, "body") or ""
        labels = _string_list(args, "labels")
        assignees = _string_list(args, "assignees")
    except ValueError as exc:
        return error_result("invalid_arguments", str(exc))
    command = ["issues", "create", "--approve", "--title", title, "--body", body]
    for label in labels:
        command.extend(["--label", label])
    for assignee in assignees:
        command.extend(["--assignee", assignee])
    return _issue_target_command(args, command)


def git_bbq_issue_edit(args: dict, **kwargs) -> str:
    del kwargs
    try:
        number = _issue_number(args)
        command = ["issues", "edit", "--approve", "--number", str(number)]
        for name, flag in (("title", "--title"), ("body", "--body"), ("state", "--state"), ("reason", "--reason")):
            value = _optional_issue_text(args, name, allow_empty=name == "body")
            if value is not None:
                command.extend([flag, value])
    except ValueError as exc:
        return error_result("invalid_arguments", str(exc))
    return _issue_target_command(args, command)


def git_bbq_issue_labels(args: dict, **kwargs) -> str:
    del kwargs
    try:
        number = _issue_number(args)
        add = _string_list(args, "add")
        remove = _string_list(args, "remove")
        if (not add) == (not remove):
            raise ValueError("provide exactly one of add or remove labels")
    except ValueError as exc:
        return error_result("invalid_arguments", str(exc))
    command = ["issues", "labels", "--approve", "--number", str(number)]
    for label in add:
        command.extend(["--add", label])
    for label in remove:
        command.extend(["--remove", label])
    return _issue_target_command(args, command)


def git_bbq_issue_assign(args: dict, **kwargs) -> str:
    del kwargs
    try:
        number = _issue_number(args)
        assignees = _string_list(args, "assignees")
        if not assignees:
            raise ValueError("at least one assignee is required")
    except ValueError as exc:
        return error_result("invalid_arguments", str(exc))
    command = ["issues", "assign", "--approve", "--number", str(number)]
    for assignee in assignees:
        command.extend(["--assignee", assignee])
    return _issue_target_command(args, command)


def git_bbq_issue_comment(args: dict, **kwargs) -> str:
    del kwargs
    try:
        number = _issue_number(args)
        body = _required_string(args, "body")
    except ValueError as exc:
        return error_result("invalid_arguments", str(exc))
    return _issue_target_command(args, ["issues", "comment", "--approve", "--number", str(number), "--body", body])


def _issue_state_tool(args: dict, state: str) -> str:
    try:
        number = _issue_number(args)
        reason = _optional_string(args, "reason")
    except ValueError as exc:
        return error_result("invalid_arguments", str(exc))
    command = ["issues", "close" if state == "closed" else "reopen", "--approve", "--number", str(number)]
    if reason is not None:
        command.extend(["--reason", reason])
    return _issue_target_command(args, command)


def git_bbq_issue_close(args: dict, **kwargs) -> str:
    del kwargs
    return _issue_state_tool(args, "closed")


def git_bbq_issue_reopen(args: dict, **kwargs) -> str:
    del kwargs
    return _issue_state_tool(args, "open")


def git_bbq_issue_subissue(args: dict, **kwargs) -> str:
    del kwargs
    try:
        parent, child = _issue_number(args, "parent"), _issue_number(args, "child")
    except ValueError as exc:
        return error_result("invalid_arguments", str(exc))
    return _issue_target_command(args, ["issues", "subissue", "--approve", "--parent", str(parent), "--child", str(child)])


def git_bbq_issue_block(args: dict, **kwargs) -> str:
    del kwargs
    try:
        issue, blocker = _issue_number(args, "issue"), _issue_number(args, "by")
    except ValueError as exc:
        return error_result("invalid_arguments", str(exc))
    return _issue_target_command(args, ["issues", "block", "--approve", "--issue", str(issue), "--by", str(blocker)])
