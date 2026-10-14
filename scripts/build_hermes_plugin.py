#!/usr/bin/env python3
"""Build the Hermes-native Git-BBQ plugin and its pinned Go runtimes."""

from __future__ import annotations

import argparse
from dataclasses import dataclass
import io
import json
import os
import platform
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
PLUGIN_SOURCE = ROOT / "plugins/git-bbq"
DEFAULT_VERSION = "0.1.0"
SKILLS_REPOSITORY = "https://github.com/mattpocock/skills.git"
CURATION_FILENAME = "git-bbq-curation.json"
SELECTION_MANIFEST_FILENAME = "skills/matt-skills-manifest.json"
SKILL_NAME_PATTERN = re.compile(r"^[a-z0-9]+(?:-[a-z0-9]+)*$")
VERSION_PATTERN = re.compile(r"^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$")
STABLE_SKILL_CATEGORIES = ("engineering", "productivity")
TARGETS = {
    "windows-x86_64": ("windows", "amd64", "git-bbq.exe"),
    "aarch64-darwin": ("darwin", "arm64", "git-bbq"),
    "x86_64-darwin": ("darwin", "amd64", "git-bbq"),
    "x86_64-linux": ("linux", "amd64", "git-bbq"),
}


@dataclass(frozen=True)
class CuratedSkill:
    """Describe an unchanged upstream skill selected for packaging."""

    source_path: str
    name: str


@dataclass(frozen=True)
class SkillSource:
    """Describe the upstream pin and root-owned package selection."""

    repository: str
    commit: str
    checkout: Path
    selection: tuple[CuratedSkill, ...]


def current_target() -> str:
    system = platform.system().lower()
    machine = platform.machine().lower()
    if system == "windows":
        return "windows-x86_64"
    if system == "darwin":
        return "aarch64-darwin" if machine in {"arm64", "aarch64"} else "x86_64-darwin"
    return "x86_64-linux"


def copy_file(source: Path, destination: Path) -> None:
    destination.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(source, destination)


def git_head(path: Path) -> str | None:
    try:
        result = subprocess.run(
            ["git", "-C", str(path), "rev-parse", "HEAD"],
            check=True,
            capture_output=True,
            text=True,
        )
    except (OSError, subprocess.CalledProcessError):
        return None
    return result.stdout.strip()


def git_output(path: Path, *arguments: str) -> str | None:
    try:
        result = subprocess.run(
            ["git", "-C", str(path), *arguments],
            check=True,
            capture_output=True,
            text=True,
        )
    except (OSError, subprocess.CalledProcessError):
        return None
    return result.stdout.strip()


def discover_stable_skill_paths(repository_root: Path) -> set[str]:
    paths: set[str] = set()
    for category in STABLE_SKILL_CATEGORIES:
        category_root = repository_root / "skills" / category
        if not category_root.is_dir():
            continue
        paths.update(
            child.relative_to(repository_root).as_posix()
            for child in category_root.iterdir()
            if child.is_dir() and (child / "SKILL.md").is_file()
        )
    return paths


def validate_curation(manifest: dict, repository_root: Path) -> list[CuratedSkill]:
    if not isinstance(manifest, dict) or manifest.get("schemaVersion") != 1:
        raise SystemExit("root curation manifest schemaVersion must be 1")
    source = manifest.get("source")
    if not isinstance(source, dict):
        raise SystemExit("root curation manifest must define its upstream source")
    if source.get("repository") != SKILLS_REPOSITORY:
        raise SystemExit(f"skill source must be the upstream repository: {SKILLS_REPOSITORY}")
    commit = source.get("commit")
    if not isinstance(commit, str) or not re.fullmatch(r"[0-9a-f]{40}", commit):
        raise SystemExit("skill source must pin a 40-character commit")

    entries = manifest.get("skills")
    if not isinstance(entries, list) or not entries:
        raise SystemExit("root curation manifest must select at least one skill")

    selection: list[CuratedSkill] = []
    seen_paths: set[str] = set()
    seen_names: set[str] = set()
    for entry in entries:
        if not isinstance(entry, dict):
            raise SystemExit("root curation entries must be objects")
        source_path = entry.get("path")
        name = entry.get("name")
        if (
            not isinstance(source_path, str)
            or not source_path.startswith("skills/")
            or Path(source_path).is_absolute()
            or ".." in Path(source_path).parts
            or Path(source_path).as_posix() != source_path
            or len(Path(source_path).parts) != 3
        ):
            raise SystemExit(f"root curation entry has an invalid upstream path: {source_path!r}")
        if source_path in seen_paths:
            raise SystemExit(f"root curation manifest has a duplicate upstream path: {source_path}")
        if not isinstance(name, str) or not SKILL_NAME_PATTERN.fullmatch(name):
            raise SystemExit(f"root curation entry has an invalid upstream name: {name!r}")
        if name != Path(source_path).name:
            raise SystemExit(f"skill name must preserve the upstream directory name: {source_path}")
        if name in seen_names:
            raise SystemExit(f"root curation manifest has a duplicate skill name: {name}")
        if not (repository_root / source_path / "SKILL.md").is_file():
            raise SystemExit(f"selected upstream skill is missing SKILL.md: {source_path}")
        seen_paths.add(source_path)
        seen_names.add(name)
        selection.append(CuratedSkill(source_path=source_path, name=name))

    discovered = discover_stable_skill_paths(repository_root)
    selected = {item.source_path for item in selection}
    if selected != discovered:
        missing = sorted(discovered - selected)
        stale = sorted(selected - discovered)
        details = []
        if missing:
            details.append(f"unselected stable skills: {', '.join(missing)}")
        if stale:
            details.append(f"paths outside the stable selection: {', '.join(stale)}")
        raise SystemExit("root curation manifest does not match its stable-skill policy (" + "; ".join(details) + ")")
    return selection


def load_pinned_source(checkout: Path) -> SkillSource:
    manifest_path = ROOT / CURATION_FILENAME
    try:
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise SystemExit(f"could not read root skill curation manifest {manifest_path}: {exc}") from exc
    selection = validate_curation(manifest, checkout)
    source = manifest["source"]
    return SkillSource(
        repository=source["repository"],
        commit=source["commit"],
        checkout=checkout,
        selection=tuple(selection),
    )


def copy_matt_skills(output: Path) -> None:
    manifest_path = ROOT / CURATION_FILENAME
    try:
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise SystemExit(f"could not read root skill curation manifest {manifest_path}: {exc}") from exc
    source = manifest.get("source", {})
    repository = source.get("repository")
    commit = source.get("commit")
    if repository != SKILLS_REPOSITORY or not isinstance(commit, str):
        raise SystemExit("root curation manifest does not pin the upstream Matt Pocock repository")

    checkout = ROOT / ".agents/skills"
    if not checkout.is_dir():
        raise SystemExit("pinned upstream skills checkout is missing; initialize the existing submodule explicitly")
    if git_head(checkout) != commit:
        raise SystemExit(f"upstream skill submodule is not at pinned commit {commit}; update the existing submodule explicitly")
    remote = git_output(checkout, "remote", "get-url", "origin")
    if remote != repository:
        raise SystemExit(f"upstream skill submodule origin must be {repository}; found {remote!r}")
    dirty = git_output(checkout, "status", "--porcelain", "--untracked-files=all")
    if dirty is None:
        raise SystemExit(f"could not inspect upstream skill submodule: {checkout}")
    if dirty:
        raise SystemExit("upstream skill submodule must be clean before package build")

    source_spec = load_pinned_source(checkout)
    with tempfile.TemporaryDirectory(prefix="git-bbq-skills-export-") as temporary:
        exported = export_pinned_selection(source_spec, Path(temporary))
        copy_skill_directories(exported, output)


def export_pinned_selection(source_spec: SkillSource, destination: Path) -> SkillSource:
    """Export only committed files from the pinned upstream Git tree."""
    destination.mkdir(parents=True, exist_ok=True)
    paths = [item.source_path for item in source_spec.selection]
    license_result = subprocess.run(
        ["git", "-C", str(source_spec.checkout), "cat-file", "-e", f"{source_spec.commit}:LICENSE"],
        check=False,
        capture_output=True,
    )
    if license_result.returncode == 0:
        paths.append("LICENSE")
    try:
        archive = subprocess.run(
            ["git", "-C", str(source_spec.checkout), "archive", "--format=tar", source_spec.commit, *paths],
            check=True,
            capture_output=True,
        ).stdout
    except (OSError, subprocess.CalledProcessError) as exc:
        raise SystemExit(f"could not export pinned upstream skills: {exc}") from exc
    try:
        with tarfile.open(fileobj=io.BytesIO(archive), mode="r:") as bundle:
            for member in bundle.getmembers():
                member_path = Path(member.name)
                if member_path.is_absolute() or ".." in member_path.parts:
                    raise SystemExit(f"pinned source archive contains an unsafe path: {member.name}")
            bundle.extractall(destination, filter="data")
    except (tarfile.TarError, OSError) as exc:
        raise SystemExit(f"could not unpack pinned upstream skills: {exc}") from exc
    return SkillSource(
        repository=source_spec.repository,
        commit=source_spec.commit,
        checkout=destination,
        selection=source_spec.selection,
    )


def copy_skill_directories(source_spec: SkillSource, output: Path) -> None:
    destination_root = output / "skills"
    destination_root.mkdir(parents=True, exist_ok=True)
    write_selection_manifest(output, source_spec)
    upstream_license = source_spec.checkout / "LICENSE"
    if upstream_license.is_file():
        copy_file(upstream_license, output / "licenses/mattpocock-skills/LICENSE")
    for item in source_spec.selection:
        source_directory = source_spec.checkout / item.source_path
        destination = destination_root / item.name
        if destination.exists():
            raise SystemExit(f"upstream skill name collides with a bundled Git-BBQ skill: {item.name}")
        shutil.copytree(source_directory, destination)


def write_selection_manifest(output: Path, source_spec: SkillSource) -> None:
    records = [
        {
            "sourcePath": item.source_path,
            "name": item.name,
            "repository": source_spec.repository,
            "commit": source_spec.commit,
        }
        for item in source_spec.selection
    ]
    manifest = {
        "$schema": "https://github.com/averyfreeman/git-bbq-hermes/blob/main/schemas/gitbbq/matt-skills-manifest.schema.json",
        "repository": source_spec.repository,
        "commit": source_spec.commit,
        "skills": records,
    }
    path = output / SELECTION_MANIFEST_FILENAME
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")


def set_plugin_version(plugin_manifest: Path, version: str) -> None:
    if not VERSION_PATTERN.fullmatch(version):
        raise SystemExit(f"invalid plugin version: {version!r}")
    content = plugin_manifest.read_text(encoding="utf-8")
    updated, count = re.subn(r"(?m)^version:\s*\S+\s*$", f"version: {version}", content, count=1)
    if count != 1:
        raise SystemExit("plugin.yaml must contain exactly one top-level version field")
    plugin_manifest.write_text(updated, encoding="utf-8")


def build(args: argparse.Namespace) -> Path:
    requested_target = args.target or current_target()
    targets = sorted(TARGETS) if requested_target == "all" else [requested_target]
    for target in targets:
        if target not in TARGETS:
            choices = ", ".join(["all", *sorted(TARGETS)])
            raise SystemExit(f"unsupported target {target!r}; choose from {choices}")

    output_arg = Path(args.output).expanduser()
    output = (ROOT / output_arg).resolve() if not output_arg.is_absolute() else output_arg.resolve()
    try:
        output.relative_to(ROOT)
    except ValueError as exc:
        raise SystemExit("package output must remain inside the repository") from exc
    package_roots = (ROOT / "dist", ROOT / ".tmp")
    allowed_output = False
    for package_root in package_roots:
        if package_root.is_symlink():
            continue
        resolved_root = package_root.resolve()
        if output != resolved_root and resolved_root in output.parents:
            allowed_output = True
            break
    if not allowed_output or output == PLUGIN_SOURCE or PLUGIN_SOURCE in output.parents:
        raise SystemExit("package output must be a generated package directory under dist/ or .tmp/")
    if output.exists():
        if not args.force:
            raise SystemExit(f"output already exists; pass --force to replace generated path: {output}")
        shutil.rmtree(output)
    output.mkdir(parents=True)
    shutil.copytree(
        PLUGIN_SOURCE,
        output,
        dirs_exist_ok=True,
        ignore=shutil.ignore_patterns("runtime", "__pycache__", ".pytest_cache", "tests"),
    )
    set_plugin_version(output / "plugin.yaml", args.plugin_version)
    copy_file(ROOT / "assets/logo.svg", output / "assets/logo.svg")
    copy_file(ROOT / "THIRD_PARTY_NOTICES.md", output / "THIRD_PARTY_NOTICES.md")
    copy_matt_skills(output)

    runtime_paths = []
    for target in targets:
        goos, goarch, executable = TARGETS[target]
        runtime_path = output / "runtime/bin" / target / executable
        runtime_path.parent.mkdir(parents=True, exist_ok=True)
        environment = os.environ.copy()
        environment.update({"GOOS": goos, "GOARCH": goarch, "CGO_ENABLED": "0"})
        command = [
            args.go,
            "build",
            "-trimpath",
            "-ldflags",
            f"-X=github.com/averyfreeman/git-bbq/internal/gitbbq.ToolVersion={args.plugin_version}",
            "-o",
            str(runtime_path),
            "./cmd/git-bbq",
        ]
        subprocess.run(command, cwd=ROOT, env=environment, check=True)
        runtime_path.chmod(0o755)
        runtime_paths.append(runtime_path)

    if not (output / "plugin.yaml").is_file() or not (output / "__init__.py").is_file():
        raise SystemExit("native Hermes plugin manifest or entrypoint was not packaged")
    if any((output / item).exists() for item in ("plugin.json", ".codex-plugin", "hooks")):
        raise SystemExit("Hermes package contains a retired portable manifest or lifecycle hook path")
    if not all(path.is_file() for path in runtime_paths):
        raise SystemExit("one or more Go runtime targets were not built")
    return output


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", default="dist/hermes/git-bbq")
    parser.add_argument("--plugin-version", default=DEFAULT_VERSION)
    parser.add_argument("--target", choices=["all", *sorted(TARGETS)])
    parser.add_argument("--go", default="go")
    parser.add_argument("--force", action="store_true")
    args = parser.parse_args()
    output = build(args)
    print(f"Git-BBQ Hermes plugin built: {output}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
