# Git BBQ development

## Repository boundaries

- `cmd/git-bbq` is the public CLI.
- `internal/gitbbq` owns scaffolding, ADRs, projections, validation, GitHabits
  planning, GitHub Issues operations, and approved execution.
- `plugins/git-bbq` contains the native Hermes manifest, typed tools, and bundled
  shared skills.
- `scripts/build_hermes_plugin.py` packages the plugin with selected upstream
  skills and a target-specific Go runtime.
- `scripts/test_hermes_plugin.py` and `scripts/test_hermes_builder.py` cover
  plugin contracts, package construction, and upstream skill curation.

The plugin packages selected skills from its pinned upstream Matt Pocock
submodule. Package selection and adapter code live here; upstream skill
content does not.

## Checks

```sh
test -z "$(gofmt -l cmd internal)"
go test ./...
go vet ./...
go build ./cmd/git-bbq
go run ./cmd/git-bbq schema .
python3 -m unittest scripts.test_hermes_plugin scripts.test_hermes_builder -v
python3 scripts/build_hermes_plugin.py --output .tmp/git-bbq-hermes --target x86_64-linux --force
hermes plugins doctor --ci .tmp/git-bbq-hermes
hermes plugins validate --json .tmp/git-bbq-hermes
```

## Skill packaging workflow

Package changes follow this order:

1. Read the pinned upstream repository and commit from
   `git-bbq-curation.json`; verify `.agents/skills` points to that clean
   upstream checkout.
2. Require the curation selection to match every skill in the stable
   `engineering` and `productivity` categories at that commit.
3. Export selected skill directories from the pinned Git tree and package them
   under their upstream directory names.
4. Preserve upstream skill directories and all committed support files without
   rewriting their instructions or frontmatter.
5. Validate the Hermes-native manifest, registered typed tools, shared skills,
   package runtime, and absence of retired portable manifests and lifecycle
   hooks with Hermes tooling.

The package build accepts the `.agents/skills` checkout only when its Git
`HEAD`, origin, and clean working tree match the upstream pin. It exports the
selected files from that Git commit, so local ignored files cannot enter the
package. The builder fails if the submodule is absent or out of sync; update the
existing submodule explicitly rather than auto-cloning. The generated
`skills/matt-skills-manifest.json` records each packaged source path, upstream
name, repository, and commit. Update the curation manifest and submodule pin
together when adopting another upstream revision.
The optional `lavish-axi` CLI may be used to review HTML architecture or
prototype artifacts, but it is not a package dependency.

For a Linux Hermes package:

```sh
python3 scripts/build_hermes_plugin.py \
  --output .tmp/git-bbq-hermes \
  --target x86_64-linux \
  --force
hermes plugins doctor --ci .tmp/git-bbq-hermes
hermes plugins validate --json .tmp/git-bbq-hermes
```

Release builds use the same builder with `--target all` or an individual target
(`aarch64-darwin`, `x86_64-darwin`, `x86_64-linux`, or `windows-x86_64`). Hermes
validation is run against each package before release artifacts are uploaded.
