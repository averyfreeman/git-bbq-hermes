# Local Hermes plugin build and installation

Build the Hermes-native package from the repository root. The builder compiles
the canonical Go CLI for the selected platform and includes it under the
package's `runtime/bin/` directory.

```sh
python3 scripts/build_hermes_plugin.py \
  --output .tmp/git-bbq-hermes \
  --target x86_64-linux \
  --force

hermes plugins doctor --ci .tmp/git-bbq-hermes
hermes plugins validate --json .tmp/git-bbq-hermes
```

Choose one of `windows-x86_64`, `aarch64-darwin`, `x86_64-darwin`, or
`x86_64-linux` for `--target`; use `all` to build every supported runtime.
Hermes and Go must be available on the build machine. Package validation checks
the manifest, registrations, and plugin security rules. The Go and Python test
commands are in `docs/DEVELOPMENT.md`.

## Install a local build

Use a runtime target that matches the host. Install the generated directory
under the active Hermes home (`$HERMES_HOME`, or `~/.hermes` when unset). Do not
overwrite an existing plugin directory without first reviewing it.

```sh
plugin_home="${HERMES_HOME:-$HOME/.hermes}/plugins"
test ! -e "$plugin_home/git-bbq" || {
  printf '%s\n' "Plugin already exists: $plugin_home/git-bbq" >&2
  exit 1
}
mkdir -p "$plugin_home"
cp -a .tmp/git-bbq-hermes "$plugin_home/git-bbq"
hermes plugins enable git-bbq
hermes plugins doctor --ci "$plugin_home/git-bbq"
```

Restart the active Hermes process if it was already running. The package uses
the Go binary bundled for its build target; it does not silently use a system
binary or download one later. Build a new package and replace the installed copy
through a deliberate update when upgrading.

## Install a GitHub release package

Release assets contain the plugin and its matching Go runtime, so users do not
need Go installed. Select the asset for the host, download the archive and
`SHA256SUMS` from the same GitHub release, verify the checksum, and extract it
directly into the active Hermes profile's plugin directory:

```sh
version=0.1.0
target=x86_64-linux # aarch64-darwin, x86_64-darwin, or windows-x86_64
archive="git-bbq-${target}.tar.gz"
release_url="https://github.com/averyfreeman/git-bbq-hermes/releases/download/v${version}"
download_dir="${TMPDIR:-/tmp}/git-bbq-${version}"
plugin_home="${HERMES_HOME:-$HOME/.hermes}/plugins"

mkdir -p "$download_dir"
curl -fsSL "$release_url/$archive" -o "$download_dir/$archive"
curl -fsSL "$release_url/SHA256SUMS" -o "$download_dir/SHA256SUMS"
(cd "$download_dir" && grep " $archive$" SHA256SUMS | sha256sum -c -)

test ! -e "$plugin_home/git-bbq" || {
  printf '%s\n' "Plugin already exists: $plugin_home/git-bbq" >&2
  exit 1
}
mkdir -p "$plugin_home/git-bbq"
tar -xzf "$download_dir/$archive" -C "$plugin_home/git-bbq"
runtime="$plugin_home/git-bbq/runtime/bin/$target/git-bbq"
test "$target" != windows-x86_64 || runtime="$runtime.exe"
if command -v gh >/dev/null 2>&1; then
  source_digest="$(tr -d '\n' < "$plugin_home/git-bbq/BUILD_SOURCE_SHA")"
  gh attestation verify "$runtime" \
    --repo averyfreeman/git-bbq-hermes \
    --source-digest "$source_digest"
else
  printf '%s\n' "Install gh to verify the runtime build attestation." >&2
fi
hermes plugins enable git-bbq
hermes plugins doctor --ci "$plugin_home/git-bbq"
```

The release workflow attests each Go executable to the public GitHub Actions
build and records its source commit in `BUILD_SOURCE_SHA`. The optional
attestation verification step requires the GitHub CLI; keep the archive checksum
check even when attestation verification is unavailable. On Windows, use a
POSIX-compatible shell with `curl`, `sha256sum`, `tar`, and `gh`, or verify the
listed checksum and attestation with equivalent local utilities before
extracting `git-bbq-windows-x86_64.tar.gz`.

For development, the repository plugin source is in `plugins/git-bbq/`, while
the built package is in `.tmp/`. Validate the source with `hermes plugins doctor
--ci plugins/git-bbq` and `hermes plugins validate --json plugins/git-bbq`; use
the built package for runtime execution so its bundled Go binary is present.

## Safety and troubleshooting

- Local file and Git tools operate only on the selected existing workspace; the
  plugin never auto-clones a repository.
- Git and other mutating tools require explicit workflow approval, and the Go
  executor retains its `--approve` gate.
- GitHub Issues tools derive the repository from the selected workspace. Without
  one, provide an explicit owner and repository. Authentication remains owned by
  `gh` or the active Hermes profile environment; never put a token in chat or
  repository files.
- A “bundled runtime is missing” error means the package was built for a
  different target, or the source directory was installed instead of the built
  package. Rebuild for the host and validate the package before installing.
- A project-local plugin under `.hermes/plugins/` is disabled by default in
  Hermes. Enable it only for a trusted repository by setting
  `HERMES_ENABLE_PROJECT_PLUGINS=true` before starting Hermes.

The release workflow builds platform-specific packages. A Hermes catalog entry
is a separate reviewed distribution path; see `PLUGIN_SUBMISSION.md` for its
current prerequisites.
