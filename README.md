# OrgTop

[![CI](https://github.com/fmueller/orgtop/actions/workflows/ci.yml/badge.svg)](https://github.com/fmueller/orgtop/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/fmueller/orgtop)](https://github.com/fmueller/orgtop/releases/latest)
[![Go Version](https://img.shields.io/github/go-mod/go-version/fmueller/orgtop)](go.mod)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue)](LICENSE)

`top` for your engineering organization: a terminal-native view of what is
happening across the repositories you care about, without opening a browser
dashboard.

OrgTop shows recent activity for an explicitly selected set of GitHub
repositories in two views: an Overview of per-repository counts and a Stream of
individual events. It reads; it never writes to GitHub, and it stores no
credential of its own.

## Installation

Download the archive for your platform from the
[latest release](https://github.com/fmueller/orgtop/releases/latest), or install
with Go:

```bash
go install github.com/fmueller/orgtop/cmd/orgtop@latest
```

A `go install` build reports `dev` for `--version`; only release archives carry
the tag. Build from source with `task build` or `go build ./cmd/orgtop`; see
[CONTRIBUTING.md](CONTRIBUTING.md) for the development setup.

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
identified before it is configured.

```console
$ orgtop --version
orgtop 0.1.0
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

An authenticated token gets 5000 GitHub REST requests per hour. OrgTop spends
one request per selected repository per refresh, so a selection stays well
inside the budget at the 60-second poll floor; a very large selection is bounded
by that hourly limit rather than by OrgTop.

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

## Contributing

Run `task check` before opening a pull request: it is the full local gate and
mirrors the CI `checks` job step for step. Build requirements, commit-message
policy, the Taskrail tracked-work flow, CI lanes, and the release process are in
[CONTRIBUTING.md](CONTRIBUTING.md). [`AGENTS.md`](AGENTS.md) is the
authoritative guide for coding agents. Release notes live in
[`CHANGELOG.md`](CHANGELOG.md).

## License

OrgTop is licensed under the Apache License 2.0. See [`LICENSE`](LICENSE).
