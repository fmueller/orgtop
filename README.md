# OrgTop

OrgTop is a terminal-native engineering activity awareness tool: `top` for your
engineering organization. It is intended to provide a compact view of current
activity across selected repositories without requiring a browser dashboard.

OrgTop v0.1 shows recent activity for an explicitly selected set of GitHub
repositories in two views: an Overview of per-repository counts and a Stream of
individual events.

## Usage

```bash
orgtop --repo owner/repository [--repo owner/repository ...]
```

Repeat `--repo` to select several repositories:

```bash
orgtop --repo acme/backend --repo acme/frontend
```

Every repository is named exactly as `owner/repository`. There is no glob or
organization-wide selection: a launch without `--repo`, with a malformed
identifier, or with glob syntax exits before the terminal UI with usage and a
concise cause, and makes no network request.

`--version` (or `-v`) prints the release version on stdout and exits, and
`--help` (or `-h`) prints usage and exits. Neither needs a `--repo` selection or
a credential, and neither makes a network request, so a downloaded binary can be
identified before it is configured. A build that was not produced by a release
reports `dev`.

```bash
orgtop --version
```

### Authentication

OrgTop uses an existing local GitHub credential and never stores one of its own.
It resolves the first available of:

1. `GH_TOKEN`
2. `GITHUB_TOKEN`
3. `gh auth token --hostname github.com`

A non-empty environment variable is used as-is and no `gh` process is started.
If none of the three yields a token, startup exits non-zero and recommends
setting `GH_TOKEN` or running `gh auth login`. The token value never appears in
output, errors, or rendered views.

### Polling, not live

OrgTop reads GitHub's repository events endpoint on an interval and honours the
poll delay GitHub advertises, with a 60-second floor. The header therefore shows
a constant `POLLING` label: data is as recent as the last completed refresh and
is not live. GitHub's events endpoint is itself cached and near-current rather
than instantaneous, so a very fresh event can take a moment to appear.

A refresh is atomic across the whole selection. The header carries a separate
freshness marker beside `POLLING`:

| Marker | Meaning |
|---|---|
| `LOADING` | The first refresh has not completed yet. |
| *(none)* | The shown snapshot is the latest complete success. |
| `ERROR` | No refresh has ever succeeded; the cause is shown. |
| `STALE` | A later refresh failed; the last successful snapshot stays visible with its last-success time and the cause. |

### Stream columns

Stream lists events newest first under a heading row that stays on screen while
the list scrolls. A narrow terminal shortens the headings and the category
names, so both spellings are listed:

| Column | Narrow | Meaning |
|---|---|---|
| `age` | `age` | How long before the last successful refresh the event happened |
| `repository` | `repo` | The repository the event belongs to |
| `category` | `type` | The event category as text: `push`, `pull request`, `review`, `comment`, or `other`, shortened to `pr`, `rev`, `com`, and `oth` |
| `actor · description` | `detail` | Who acted, and a concise description of what the event did |

Times are ages, not clock times. A row reading `3d` is the event's
age at the last successful refresh the header reports, so it stays put while a
later failed refresh leaves the header `STALE`.

Above the headings Stream states how much activity it is showing: the number of
events in the current snapshot and, when the 500-event bound discarded older
activity, that the list stops at that limit. Content too wide for the terminal
ends in `…`, marking that row as shortened rather than complete.

### Controls

| Key | Action |
|---|---|
| `1` | Open Overview |
| `2` | Open Stream |
| `tab` | Toggle between the two views |
| `up` / `down` | Scroll the active view by one row |
| `pgup` / `pgdown` | Scroll the active view by one page |
| `q` | Quit |
| `ctrl+c` | Quit |

Quitting cancels whatever refresh is in flight and restores the terminal.

## Prerequisites

- Go 1.26.7 or a compatible newer release
- [Task](https://taskfile.dev/) for the convenience commands
- Taskrail for tracked specifications and work
- [mise](https://mise.jdx.dev/) optionally provisions the pinned toolchain

Run `mise install` to install the versions declared in `mise.toml`, or provide the
tools through another mechanism.

## Installation

There is no published release yet. Build from source:

```bash
task build     # or: go build ./cmd/orgtop
./orgtop --repo acme/backend
```

`task build` and the release build both produce a single static `orgtop`
executable from `./cmd/orgtop`.

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
`docs/changelog.md` is the authoring policy for entries.
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
