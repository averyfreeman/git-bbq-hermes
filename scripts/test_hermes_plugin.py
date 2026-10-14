"""Contract tests for the native Hermes Git-BBQ plugin."""

from __future__ import annotations

import importlib.util
import json
import os
from pathlib import Path
import sys
import tempfile
import types
import unittest
from unittest import mock


ROOT = Path(__file__).resolve().parents[1]
PLUGIN_DIR = ROOT / "plugins/git-bbq"
PLUGIN_MODULE = "git_bbq_hermes_contract_test"


class FakeContext:
    def __init__(self):
        self.tools = {}
        self.commands = {}
        self.skills = []
        self.hooks = {}

    def register_tool(self, **kwargs):
        self.tools[kwargs["name"]] = kwargs

    def register_command(self, name, handler, **kwargs):
        self.commands[name] = {"handler": handler, **kwargs}

    def register_skill(self, name, path):
        self.skills.append((name, Path(path)))

    def register_hook(self, event, callback):
        self.hooks[event] = callback


def load_plugin():
    init_path = PLUGIN_DIR / "__init__.py"
    spec = importlib.util.spec_from_file_location(
        PLUGIN_MODULE,
        init_path,
        submodule_search_locations=[str(PLUGIN_DIR)],
    )
    if spec is None or spec.loader is None:
        raise AssertionError(f"native plugin entrypoint is missing: {init_path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[PLUGIN_MODULE] = module
    spec.loader.exec_module(module)
    return module


class NativePluginTests(unittest.TestCase):
    def test_registers_typed_project_tool_and_shared_slash_command(self):
        manifest = (PLUGIN_DIR / "plugin.yaml").read_text(encoding="utf-8")
        self.assertIn("name: git-bbq", manifest)
        self.assertIn("  - git_bbq_project", manifest)

        plugin = load_plugin()
        context = FakeContext()
        plugin.register(context)
        self.assertIn("git_bbq_project", context.tools)
        self.assertIn("git-bbq", context.commands)
        self.assertIn("pre_tool_call", context.hooks)
        self.assertTrue(context.skills)

        with tempfile.TemporaryDirectory(prefix="git-bbq-plugin-test-") as workspace:
            result = types.SimpleNamespace(returncode=0, stdout='{"adr_count":3}', stderr="")
            with (
                mock.patch.object(plugin.tools, "_resolve_cli_binary", return_value=Path("/fixture/git-bbq")),
                mock.patch.object(plugin.tools.subprocess, "run", return_value=result) as run,
            ):
                tool_output = context.tools["git_bbq_project"]["handler"]({"workspace": workspace})
                command_output = context.commands["git-bbq"]["handler"]("project " + workspace)

        expected = {"ok": True, "data": {"adr_count": 3}, "error": None}
        self.assertEqual(json.loads(tool_output), expected)
        self.assertEqual(json.loads(command_output), expected)
        self.assertEqual(run.call_count, 2)
        for call in run.call_args_list:
            args, kwargs = call
            self.assertEqual(args[0], ["/fixture/git-bbq", "project", "--json"])
            self.assertEqual(kwargs["cwd"], workspace)
            self.assertIs(kwargs["shell"], False)

    def test_registers_policy_safe_local_workspace_tool_surface(self):
        plugin = load_plugin()
        context = FakeContext()
        plugin.register(context)
        expected = {
            "git_bbq_assess",
            "git_bbq_init",
            "git_bbq_validate",
            "git_bbq_project",
            "git_bbq_adr_new",
            "git_bbq_adr_index",
            "git_bbq_migrate",
            "git_bbq_githabits",
            "git_bbq_git_plan",
            "git_bbq_git_execute",
            "git_bbq_release_plan",
            "git_bbq_baseline_plan",
            "git_bbq_baseline_execute",
            "git_bbq_issues_list",
            "git_bbq_issue_get",
            "git_bbq_issue_create",
            "git_bbq_issue_edit",
            "git_bbq_issue_labels",
            "git_bbq_issue_assign",
            "git_bbq_issue_comment",
            "git_bbq_issue_close",
            "git_bbq_issue_reopen",
            "git_bbq_issue_subissue",
            "git_bbq_issue_block",
        }
        self.assertEqual(set(context.tools), expected)
        manifest = (PLUGIN_DIR / "plugin.yaml").read_text(encoding="utf-8")
        for tool_name in expected:
            self.assertIn(f"  - {tool_name}", manifest)
        self.assertIn("provides_hooks:\n  - pre_tool_call", manifest)
        self.assertEqual({name for name, _ in context.skills}, {"git-bbq", "githabits", "github-issues"})

    def test_mutations_use_host_approval_gate_per_workflow_turn(self):
        plugin = load_plugin()
        context = FakeContext()
        plugin.register(context)
        gate = context.hooks["pre_tool_call"]
        expected_mutations = {
            "git_bbq_init", "git_bbq_project", "git_bbq_adr_new", "git_bbq_adr_index",
            "git_bbq_migrate", "git_bbq_git_execute", "git_bbq_baseline_execute",
            "git_bbq_issue_create", "git_bbq_issue_edit", "git_bbq_issue_labels",
            "git_bbq_issue_assign", "git_bbq_issue_comment", "git_bbq_issue_close",
            "git_bbq_issue_reopen", "git_bbq_issue_subissue", "git_bbq_issue_block",
        }
        self.assertEqual(plugin._MUTATING_TOOLS, expected_mutations)
        for tool_name in plugin._MUTATING_TOOLS:
            with self.subTest(tool_name=tool_name):
                decision = gate(
                    tool_name=tool_name,
                    args={"apply": True} if tool_name == "git_bbq_migrate" else {},
                    session_id="session-1",
                    turn_id="turn-1",
                )
                self.assertEqual(decision["action"], "approve")
                self.assertEqual(decision["rule_key"], "git-bbq-workflow:session-1:turn-1")
        self.assertIsNone(gate(tool_name="git_bbq_validate", args={}, turn_id="turn-1"))
        self.assertIsNone(gate(tool_name="git_bbq_migrate", args={}, turn_id="turn-1"))
        self.assertEqual(
            gate(tool_name="git_bbq_migrate", args={"apply": True}, turn_id="turn-1")["action"],
            "approve",
        )
        self.assertEqual(
            gate(tool_name="git_bbq_issue_close", args={}, turn_id="")["action"],
            "block",
        )
        for name, schema in plugin.schemas.TOOLS.items():
            if name in plugin._MUTATING_TOOLS:
                self.assertNotIn("approved", schema["parameters"]["properties"])
                self.assertNotIn("approved", schema["parameters"]["required"])

    def test_read_only_validation_and_mutation_approval_contract(self):
        plugin = load_plugin()
        context = FakeContext()
        plugin.register(context)
        with tempfile.TemporaryDirectory(prefix="git-bbq-plugin-test-") as workspace:
            result = types.SimpleNamespace(returncode=0, stdout='{"valid":true}', stderr="")
            with (
                mock.patch.object(plugin.tools, "_resolve_cli_binary", return_value=Path("/fixture/git-bbq")),
                mock.patch.object(plugin.tools.subprocess, "run", return_value=result) as run,
            ):
                read_output = context.tools["git_bbq_validate"]["handler"]({"workspace": workspace})
                decision = context.hooks["pre_tool_call"](
                    tool_name="git_bbq_project", args={"workspace": workspace},
                    session_id="session", turn_id="turn",
                )
                project_output = context.tools["git_bbq_project"]["handler"]({"workspace": workspace})

        self.assertTrue(json.loads(read_output)["ok"])
        self.assertEqual(decision["action"], "approve")
        self.assertTrue(json.loads(project_output)["ok"])
        self.assertEqual(run.call_count, 2)
        self.assertEqual(run.call_args_list[0].args[0], ["/fixture/git-bbq", "validate", "--json"])
        self.assertEqual(run.call_args_list[1].args[0], ["/fixture/git-bbq", "project", "--json"])

    def test_init_arguments_are_fixed_and_approval_gated(self):
        plugin = load_plugin()
        context = FakeContext()
        plugin.register(context)
        with tempfile.TemporaryDirectory(prefix="git-bbq-plugin-test-") as workspace:
            result = types.SimpleNamespace(returncode=0, stdout='{"created":[]}', stderr="")
            with (
                mock.patch.object(plugin.tools, "_resolve_cli_binary", return_value=Path("/fixture/git-bbq")),
                mock.patch.object(plugin.tools.subprocess, "run", return_value=result) as run,
            ):
                decision = context.hooks["pre_tool_call"](
                    tool_name="git_bbq_init", args={"workspace": workspace}, turn_id="turn"
                )
                output = plugin.tools.git_bbq_init({
                    "workspace": workspace,
                    "project_name": "demo",
                    "problem": "shell; syntax stays data",
                    "languages": ["go", "python"],
                })

        self.assertEqual(decision["action"], "approve")
        self.assertTrue(json.loads(output)["ok"])
        self.assertEqual(run.call_count, 1)
        self.assertEqual(
            run.call_args.args[0],
            [
                "/fixture/git-bbq",
                "init",
                "--json",
                "--here",
                "--name",
                "demo",
                "--problem",
                "shell; syntax stays data",
                "--language",
                "go",
                "--language",
                "python",
            ],
        )
        self.assertIs(run.call_args.kwargs["shell"], False)

    def test_migrate_apply_is_explicit_and_host_gated(self):
        plugin = load_plugin()
        context = FakeContext()
        plugin.register(context)
        result = types.SimpleNamespace(returncode=0, stdout='{"changed":[]}', stderr="")
        with (
            mock.patch.object(plugin.tools, "_resolve_cli_binary", return_value=Path("/fixture/git-bbq")),
            mock.patch.object(plugin.tools.subprocess, "run", return_value=result) as run,
        ):
            preview_gate = context.hooks["pre_tool_call"](
                tool_name="git_bbq_migrate", args={}, turn_id="turn"
            )
            preview = plugin.tools.git_bbq_migrate({})
            apply_gate = context.hooks["pre_tool_call"](
                tool_name="git_bbq_migrate", args={"apply": True}, turn_id="turn"
            )
            applied = plugin.tools.git_bbq_migrate({"apply": True})

        self.assertIsNone(preview_gate)
        self.assertEqual(apply_gate["action"], "approve")
        self.assertTrue(json.loads(preview)["ok"])
        self.assertTrue(json.loads(applied)["ok"])
        self.assertEqual(run.call_args_list[0].args[0], ["/fixture/git-bbq", "migrate", "--json"])
        self.assertEqual(
            run.call_args_list[1].args[0], ["/fixture/git-bbq", "migrate", "--json", "--approve"]
        )

    def test_issue_tools_use_explicit_remote_target_and_gate_writes(self):
        plugin = load_plugin()
        context = FakeContext()
        plugin.register(context)
        result = types.SimpleNamespace(returncode=0, stdout='[]', stderr="")
        with (
            mock.patch.object(plugin.tools, "_resolve_cli_binary", return_value=Path("/fixture/git-bbq")),
            mock.patch.object(plugin.tools.subprocess, "run", return_value=result) as run,
        ):
            listing = context.tools["git_bbq_issues_list"]["handler"]({"owner": "example", "repo": "project"})
            decision = context.hooks["pre_tool_call"](
                tool_name="git_bbq_issue_create", args={"owner": "example", "repo": "project"},
                session_id="session", turn_id="turn",
            )
            created = context.tools["git_bbq_issue_create"]["handler"]({
                "owner": "example",
                "repo": "project",
                "title": "Parent spec",
                "body": "Acceptance criteria",
                "labels": ["needs-triage"],
            })

        self.assertTrue(json.loads(listing)["ok"])
        self.assertEqual(decision["action"], "approve")
        self.assertTrue(json.loads(created)["ok"])
        self.assertEqual(run.call_count, 2)
        self.assertEqual(
            run.call_args_list[0].args[0],
            ["/fixture/git-bbq", "issues", "list", "--state", "open", "--owner", "example", "--repo", "project"],
        )
        self.assertEqual(
            run.call_args_list[1].args[0],
            [
                "/fixture/git-bbq", "issues", "create", "--approve", "--title", "Parent spec",
                "--body", "Acceptance criteria", "--label", "needs-triage", "--owner", "example", "--repo", "project",
            ],
        )
        self.assertIs(run.call_args.kwargs["shell"], False)

    def test_relative_workspace_cannot_escape_current_directory(self):
        plugin = load_plugin()
        with tempfile.TemporaryDirectory(prefix="git-bbq-workspace-") as workspace:
            with tempfile.TemporaryDirectory(prefix="git-bbq-outside-") as outside:
                relative = os.path.relpath(outside, workspace)
                with mock.patch.object(Path, "cwd", return_value=Path(workspace)):
                    with self.assertRaisesRegex(ValueError, "inside the current workspace"):
                        plugin.tools._resolve_workspace(relative)


if __name__ == "__main__":
    unittest.main()
