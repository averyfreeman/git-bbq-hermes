# Hermes plugin catalog submission

This is a release-readiness checklist for a possible Hermes plugin catalog
submission. It does not claim that Git-BBQ has been submitted, accepted, or
listed.

## Current package boundary

The native plugin source is `plugins/git-bbq/`. Release packages are built by
`scripts/build_hermes_plugin.py`, which copies that source and adds a
platform-specific Go executable under `runtime/bin/`. The checked-in source
directory does not contain the bundled executable. Therefore a catalog install
that points directly at the source subdirectory is not a usable runtime
package, even though its manifest and registration contract validate.

Do not submit a catalog entry that points at the unbuilt source directory. A
catalog-ready distribution must make the correct runtime available at the exact
pinned install tree, or define and validate a supported installation mechanism
that supplies it. This is separate from the release workflow's built package
artifacts.

## Preflight for a built package

Build and run Hermes' official validators against each target package:

```sh
python3 scripts/build_hermes_plugin.py \
  --output .tmp/git-bbq-hermes \
  --target x86_64-linux \
  --force
hermes plugins doctor --ci .tmp/git-bbq-hermes
hermes plugins validate --json .tmp/git-bbq-hermes
```

Run the Go and adapter suites as well:

```sh
go test ./...
go vet ./...
python3 -m unittest scripts.test_hermes_plugin scripts.test_hermes_builder -v
```

A future catalog entry for a monorepo may point to `plugins/git-bbq/` using the
catalog's `subdir` field, but only after the runtime-install boundary above is
resolved and the exact pinned commit passes `hermes plugins validate` at that
subdirectory. Review every capability, tool declaration, dependency, security
finding, and warning; a passing local manifest check is not catalog admission.

## Release gates

- [ ] The install tree at the exact pinned commit contains or obtains the
  platform-matched Go runtime through a reviewed, supported mechanism.
- [ ] `hermes plugins validate` passes on that install tree, not only on a
  temporary package output.
- [ ] `hermes plugins doctor --ci` confirms the expected tools, command, and
  skills register.
- [ ] All supported runtime targets build and pass package validation.
- [ ] Go tests, Go vet, and Python adapter/builder tests pass.
- [ ] GitHub Issues mutations use the selected repository, required approval,
  and `needs-triage` default without exposing credentials.
- [ ] Documentation, privacy notice, terms, and third-party notices describe
  the released package accurately.
- [ ] A catalog PR pins a full immutable commit and is reviewed and merged by a
  Hermes catalog maintainer before any listing is claimed.
