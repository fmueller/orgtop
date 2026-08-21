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
task test
task format
task format-check
task lint
task vet
task check
task clean
```

The corresponding direct Go commands remain available, including `go build
./cmd/orgtop`, `go test ./...`, and `go vet ./...`.

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
