# Git-BBQ privacy notice

Git-BBQ is a local Go CLI and Hermes plugin. It does not operate a telemetry
service or send repository contents to a Git-BBQ-operated service.

The Hermes adapter runs the bundled Go executable in the selected workspace.
Project scaffolding, validation, and Git operations read or write local files
only as described by the command and policy. Mutating operations require explicit
approval.

When GitHub Issues tools are used, Git-BBQ invokes the authenticated `gh` CLI
and sends the requested issue data and operation to GitHub. Authentication is
owned by `gh` or the active Hermes profile's environment. Repository contents
are not uploaded except for text the user explicitly submits as issue content.

Building a package may read the pinned Matt Pocock skills submodule and copies
selected upstream skills into the package. The build does not contact a
Git-BBQ-operated service.

Questions about this notice can be opened as a GitHub issue in this repository:
https://github.com/averyfreeman/git-bbq-hermes/issues
