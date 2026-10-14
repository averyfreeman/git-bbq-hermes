"""Tests for building the standalone Hermes plugin package."""

from __future__ import annotations

import argparse
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest import mock


ROOT = Path(__file__).resolve().parents[1]
BUILDER_PATH = ROOT / "scripts/build_hermes_plugin.py"
BUILDER_NAME = "git_bbq_hermes_builder_test"


def load_builder():
    spec = importlib.util.spec_from_file_location(BUILDER_NAME, BUILDER_PATH)
    if spec is None or spec.loader is None:
        raise AssertionError(f"builder module is missing: {BUILDER_PATH}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[BUILDER_NAME] = module
    spec.loader.exec_module(module)
    return module


class HermesBuilderTests(unittest.TestCase):
    def test_build_contains_native_package_and_selected_runtime_only(self):
        builder = load_builder()
        generated_root = ROOT / ".tmp"
        generated_root.mkdir(exist_ok=True)
        with tempfile.TemporaryDirectory(prefix="git-bbq-hermes-builder-", dir=generated_root) as temporary:
            package = Path(temporary) / "package"

            def copy_test_skills(output: Path) -> None:
                upstream_skill = output / "skills/example-skill/SKILL.md"
                upstream_skill.parent.mkdir(parents=True)
                upstream_skill.write_text(
                    "---\nname: example-skill\ndescription: Pinned upstream skill.\n---\n\n"
                    "Keep this upstream instruction unchanged.\n",
                    encoding="utf-8",
                )
                manifest = {
                    "repository": builder.SKILLS_REPOSITORY,
                    "commit": "a" * 40,
                    "skills": [{
                        "sourcePath": "skills/engineering/example-skill",
                        "name": "example-skill",
                        "repository": builder.SKILLS_REPOSITORY,
                        "commit": "a" * 40,
                    }],
                }
                (output / "skills/matt-skills-manifest.json").write_text(
                    json.dumps(manifest), encoding="utf-8"
                )

            def fake_build(command, **kwargs):
                build_commands.append(command)
                output_index = command.index("-o") + 1
                binary = Path(command[output_index])
                binary.parent.mkdir(parents=True, exist_ok=True)
                binary.write_bytes(b"test-runtime")
                return subprocess.CompletedProcess(command, 0)

            args = argparse.Namespace(
                output=str(package),
                plugin_version="0.1.0",
                target="x86_64-linux",
                go="go",
                force=False,
            )
            build_commands = []
            with (
                mock.patch.object(builder, "copy_matt_skills", side_effect=copy_test_skills),
                mock.patch.object(builder.subprocess, "run", side_effect=fake_build),
            ):
                builder.build(args)

            self.assertTrue((package / "plugin.yaml").is_file())
            self.assertTrue((package / "__init__.py").is_file())
            self.assertTrue((package / "runtime/bin/x86_64-linux/git-bbq").is_file())
            self.assertFalse((package / "plugin.json").exists())
            self.assertFalse((package / ".codex-plugin").exists())
            self.assertFalse((package / "hooks").exists())
            self.assertTrue(
                any(
                    "github.com/averyfreeman/git-bbq/internal/gitbbq.ToolVersion=0.1.0" in value
                    for value in build_commands[0]
                ),
                f"CLI build did not receive the package version: {build_commands[0]}",
            )
            self.assertEqual(
                (package / "skills/example-skill/SKILL.md").read_text(encoding="utf-8"),
                "---\nname: example-skill\ndescription: Pinned upstream skill.\n---\n\n"
                "Keep this upstream instruction unchanged.\n",
            )

    def test_force_rejects_protected_repository_output_paths(self):
        builder = load_builder()
        for relative in (".git", ".agents/skills", ".tmp", "dist"):
            with self.subTest(relative=relative), tempfile.TemporaryDirectory(
                prefix="git-bbq-protected-output-"
            ) as temporary:
                root = Path(temporary)
                protected = root / relative
                protected.mkdir(parents=True)
                marker = protected / "keep.txt"
                marker.write_text("protected repository data\n", encoding="utf-8")
                args = argparse.Namespace(
                    output=relative,
                    plugin_version="0.1.0",
                    target="x86_64-linux",
                    go="go",
                    force=True,
                )
                with (
                    mock.patch.object(builder, "ROOT", root),
                    mock.patch.object(builder, "PLUGIN_SOURCE", root / "plugins/git-bbq"),
                    self.assertRaises(SystemExit),
                ):
                    builder.build(args)
                self.assertEqual(marker.read_text(encoding="utf-8"), "protected repository data\n")


class HermesCurationTests(unittest.TestCase):
    def setUp(self):
        self.builder = load_builder()
        self.temporary = tempfile.TemporaryDirectory(prefix="git-bbq-hermes-curation-")
        self.root = Path(self.temporary.name)
        for relative in ("skills/engineering/one", "skills/productivity/two"):
            skill = self.root / relative
            skill.mkdir(parents=True)
            (skill / "SKILL.md").write_text(
                "---\nname: fixture\ndescription: fixture\n---\n", encoding="utf-8"
            )

    def tearDown(self):
        self.temporary.cleanup()

    def manifest(self):
        return {
            "schemaVersion": 1,
            "source": {
                "repository": self.builder.SKILLS_REPOSITORY,
                "commit": "a" * 40,
            },
            "skills": [
                {"path": "skills/engineering/one", "name": "one"},
                {"path": "skills/productivity/two", "name": "two"},
            ],
        }

    def assert_invalid(self, manifest, message):
        with self.assertRaisesRegex(SystemExit, message):
            self.builder.validate_curation(manifest, self.root)

    def test_accepts_root_owned_upstream_selection(self):
        selection = self.builder.validate_curation(self.manifest(), self.root)
        self.assertEqual(
            [(item.source_path, item.name) for item in selection],
            [("skills/engineering/one", "one"), ("skills/productivity/two", "two")],
        )

    def test_rejects_non_upstream_source(self):
        manifest = self.manifest()
        manifest["source"]["repository"] = "https://github.com/example/skills.git"
        self.assert_invalid(manifest, "must be the upstream repository")

    def test_rejects_unpinned_source(self):
        manifest = self.manifest()
        manifest["source"]["commit"] = "main"
        self.assert_invalid(manifest, "40-character commit")

    def test_rejects_missing_skill_content(self):
        manifest = self.manifest()
        manifest["skills"][0]["path"] = "skills/engineering/missing"
        manifest["skills"][0]["name"] = "missing"
        self.assert_invalid(manifest, "missing SKILL.md")

    def test_rejects_duplicate_or_renamed_skill(self):
        manifest = self.manifest()
        manifest["skills"][0]["name"] = "two"
        self.assert_invalid(manifest, "preserve the upstream directory name")

    def test_rejects_path_traversal(self):
        manifest = self.manifest()
        manifest["skills"][0]["path"] = "skills/engineering/../one"
        self.assert_invalid(manifest, "invalid upstream path")

    def test_requires_complete_stable_skill_selection(self):
        extra = self.root / "skills/engineering/three"
        extra.mkdir()
        (extra / "SKILL.md").write_text(
            "---\nname: three\ndescription: fixture\n---\n", encoding="utf-8"
        )
        self.assert_invalid(self.manifest(), "unselected stable skills")


if __name__ == "__main__":
    unittest.main()
