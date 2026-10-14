"""Hermes tool schemas for Git-BBQ."""

def _tool(name, description, properties=None, required=()):
    return {
        "name": name,
        "description": description,
        "parameters": {
            "type": "object",
            "properties": properties or {},
            "required": list(required),
            "additionalProperties": False,
        },
    }


WORKSPACE = {
    "workspace": {
        "type": "string",
        "description": "Existing selected checkout; omitted means the current working directory.",
    }
}
ACTION = {
    "type": "string",
    "enum": ["init", "branch", "stage", "commit", "tag", "remote", "push"],
}
LANGUAGES = {
    "type": "array",
    "items": {"type": "string", "enum": ["go", "python", "typescript", "javascript", "rust", "java", "csharp"]},
    "minItems": 1,
    "uniqueItems": True,
}


TOOLS = {
    "git_bbq_assess": _tool("git_bbq_assess", "Preview Git-BBQ scaffold changes without writing files.", WORKSPACE),
    "git_bbq_init": _tool(
        "git_bbq_init", "Initialize Git-BBQ project records in the selected workspace.",
        {**WORKSPACE, "project_name": {"type": "string"}, "problem": {"type": "string"}, "languages": LANGUAGES},
        ("project_name", "problem", "languages"),
    ),
    "git_bbq_validate": _tool("git_bbq_validate", "Read-only validation of Git-BBQ project records and projections.", WORKSPACE),
    "git_bbq_project": _tool(
        "git_bbq_project", "Regenerate architecture projections in the selected workspace.",
        WORKSPACE,
    ),
    "git_bbq_adr_new": _tool(
        "git_bbq_adr_new", "Create an architecture decision record in the selected workspace.",
        {
            **WORKSPACE,
            "title": {"type": "string"}, "context": {"type": "string"},
            "decision": {"type": "string"}, "why": {"type": "string"},
            "status": {"type": "string", "enum": ["proposed", "accepted", "deprecated"]},
        },
        ("title", "context", "decision", "why"),
    ),
    "git_bbq_adr_index": _tool(
        "git_bbq_adr_index", "Regenerate and read the ADR retrieval index in the selected workspace.",
        WORKSPACE,
    ),
    "git_bbq_migrate": _tool(
        "git_bbq_migrate", "Preview migration by default; set apply true to execute it. Archiving legacy GitHabits also applies the migration.",
        {**WORKSPACE, "apply": {"type": "boolean"}, "archive_legacy_githabits": {"type": "boolean"}},
    ),
    "git_bbq_githabits": _tool("git_bbq_githabits", "Read the selected project's GitHabits policy.", WORKSPACE),
    "git_bbq_git_plan": _tool(
        "git_bbq_git_plan", "Create a read-only, policy-checked plan for one Git action.",
        {
            **WORKSPACE, "action": ACTION, "branch": {"type": "string"}, "message": {"type": "string"},
            "tag": {"type": "string"}, "paths": {"type": "array", "items": {"type": "string"}},
            "existing_tags": {"type": "array", "items": {"type": "string"}},
            "provision_remote": {"type": "boolean"},
        }, ("action",),
    ),
    "git_bbq_git_execute": _tool(
        "git_bbq_git_execute", "Execute one policy-checked GitHabits action after Hermes's human-approval gate.",
        {
            **WORKSPACE, "action": ACTION, "branch": {"type": "string"},
            "message": {"type": "string"}, "tag": {"type": "string"},
            "paths": {"type": "array", "items": {"type": "string"}},
            "existing_tags": {"type": "array", "items": {"type": "string"}},
            "provision_remote": {"type": "boolean"},
        }, ("action",),
    ),
    "git_bbq_release_plan": _tool(
        "git_bbq_release_plan", "Recommend a SemVer bump from commits after the latest tag; read-only.",
        {**WORKSPACE, "since": {"type": "string"}},
    ),
    "git_bbq_baseline_plan": _tool(
        "git_bbq_baseline_plan", "Plan an initial tag baseline without changing local or remote tags.", WORKSPACE,
    ),
    "git_bbq_baseline_execute": _tool(
        "git_bbq_baseline_execute", "Establish and push the policy-configured initial baseline after Hermes's human-approval gate.",
        WORKSPACE,
    ),
}

ISSUE_TARGET = {
    "workspace": {"type": "string", "description": "Selected checkout; when omitted, supply owner and repo explicitly."},
    "owner": {"type": "string", "description": "GitHub owner for remote-only operation."},
    "repo": {"type": "string", "description": "GitHub repository for remote-only operation."},
}
ISSUE_NUMBER = {"type": "integer", "minimum": 1}
STRING_LIST = {"type": "array", "items": {"type": "string"}, "minItems": 1}
TOOLS.update({
    "git_bbq_issues_list": _tool(
        "git_bbq_issues_list", "List GitHub issues, excluding pull requests; uses workspace origin or explicit owner/repo.",
        {**ISSUE_TARGET, "state": {"type": "string", "enum": ["open", "closed", "all"]}},
    ),
    "git_bbq_issue_get": _tool(
        "git_bbq_issue_get", "Read one GitHub issue by number.",
        {**ISSUE_TARGET, "number": ISSUE_NUMBER}, ("number",),
    ),
    "git_bbq_issue_create": _tool(
        "git_bbq_issue_create", "Create a GitHub issue; defaults to needs-triage. Writes use Hermes's human-approval gate.",
        {**ISSUE_TARGET, "title": {"type": "string"}, "body": {"type": "string"}, "labels": STRING_LIST, "assignees": STRING_LIST},
        ("title",),
    ),
    "git_bbq_issue_edit": _tool(
        "git_bbq_issue_edit", "Edit selected GitHub issue fields; writes use Hermes's human-approval gate.",
        {**ISSUE_TARGET, "number": ISSUE_NUMBER, "title": {"type": "string"}, "body": {"type": "string"}, "state": {"type": "string", "enum": ["open", "closed"]}, "reason": {"type": "string", "enum": ["completed", "not_planned"]}},
        ("number",),
    ),
    "git_bbq_issue_labels": _tool(
        "git_bbq_issue_labels", "Add or remove labels on a GitHub issue; supply exactly one action. Writes use Hermes's human-approval gate.",
        {**ISSUE_TARGET, "number": ISSUE_NUMBER, "add": STRING_LIST, "remove": STRING_LIST},
        ("number",),
    ),
    "git_bbq_issue_assign": _tool(
        "git_bbq_issue_assign", "Assign GitHub account logins to an issue; writes use Hermes's human-approval gate.",
        {**ISSUE_TARGET, "number": ISSUE_NUMBER, "assignees": STRING_LIST},
        ("number", "assignees"),
    ),
    "git_bbq_issue_comment": _tool(
        "git_bbq_issue_comment", "Add a comment to a GitHub issue; writes use Hermes's human-approval gate.",
        {**ISSUE_TARGET, "number": ISSUE_NUMBER, "body": {"type": "string"}},
        ("number", "body"),
    ),
    "git_bbq_issue_close": _tool(
        "git_bbq_issue_close", "Close a GitHub issue; writes use Hermes's human-approval gate.",
        {**ISSUE_TARGET, "number": ISSUE_NUMBER, "reason": {"type": "string", "enum": ["completed", "not_planned"]}},
        ("number",),
    ),
    "git_bbq_issue_reopen": _tool(
        "git_bbq_issue_reopen", "Reopen a GitHub issue; writes use Hermes's human-approval gate.",
        {**ISSUE_TARGET, "number": ISSUE_NUMBER}, ("number",),
    ),
    "git_bbq_issue_subissue": _tool(
        "git_bbq_issue_subissue", "Link a child issue beneath its parent using GitHub's native relationship; writes use Hermes's human-approval gate.",
        {**ISSUE_TARGET, "parent": ISSUE_NUMBER, "child": ISSUE_NUMBER},
        ("parent", "child"),
    ),
    "git_bbq_issue_block": _tool(
        "git_bbq_issue_block", "Record a native blocked-by dependency; writes use Hermes's human-approval gate.",
        {**ISSUE_TARGET, "issue": ISSUE_NUMBER, "by": ISSUE_NUMBER},
        ("issue", "by"),
    ),
})
