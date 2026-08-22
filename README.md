# OrgTop

OrgTop is a terminal-native engineering activity awareness tool: `top` for your
engineering organization. It is intended to provide a compact view of current
activity across selected repositories without requiring a browser dashboard.

OrgTop is in early development. The repository currently provides the project and
planning foundation; the v0.1 product functionality is not implemented yet.

## Prerequisites

- Go 1.26.7 or a compatible newer release
- [Task](https://taskfile.dev/) for the convenience commands
- Taskrail for tracked specifications and work
- [mise](https://mise.jdx.dev/) optionally provisions the pinned toolchain

Run `mise install` to install the versions declared in `mise.toml`, or provide the
tools through another mechanism.

## Development

```bash
task build
task build:cross
task test
task run:smoke
task fmt
task fmt:check
task lint
task vet
task licenses:check
task workflow:validate
task check
task clean
task hooks-install
task test:mutate
```

`task check` is the full local gate and mirrors the CI `checks` job step for step.
The corresponding direct Go commands remain available, including `go build
./cmd/orgtop`, `go test ./...`, and `go vet ./...`.

`task hooks-install` enables the opt-in Lefthook checks for formatting, vet,
Taskrail validation, commit-message policy, and pre-push tests. CI remains the
authoritative gate.

`task test:mutate` runs differential Gremlins mutation testing against `main`
(`BASE=<ref>` overrides it). The full `task test:mutate:gate` runs weekly in CI
and on manual dispatch rather than on every pull request.

## Continuous integration

Two lanes split the work by what changed:

- `CI` runs formatting, vet, lint, licenses, Taskrail validation, and release-config
  checks, then a build/test matrix across Linux (x86-64 and arm64), Windows, and
  macOS, plus a cross-compile smoke over every platform the release publishes.
  Pull requests run the matrix on Linux only for fast feedback; pushes to `main`
  and manual dispatch run all of it.
- `Planning checks` is the fast lane for planning, spec, doc, and agent-skill
  changes. It keeps the Taskrail and skill-mirror gates without starting the
  matrix. Its trigger paths are the exact mirror of the paths `CI` ignores.

Mutation testing runs weekly and on demand in its own workflow, never on a pull
request.

## Releases

`CHANGELOG.md` follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
is the source of release notes. Add a `## [<version>]` section before tagging: the
release workflow refuses to publish a tag whose section is missing or empty.
Pushing a `v*` tag builds and publishes through GoReleaser; a manual dispatch
builds a local snapshot and publishes nothing.

## Taskrail

Versioned product contracts live in `specs/`, and implementation tasks live in
`planning/tasks/`. Before selecting work, run:

```bash
taskrail validate
taskrail status
taskrail next
```

Use the Taskrail lifecycle to start, complete, and verify tracked work. Do not edit
`planning/STATE.md` execution fields manually.

## License

OrgTop is licensed under the Apache License 2.0. See `LICENSE`.
