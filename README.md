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

OrgTop is one standalone executable. Every channel below redistributes the
byte-identical executable published by this repository for a tag; none of them
is a runtime dependency, and none changes the command, its arguments, or how it
resolves a token.

**Release archives** — the complete installation path, and the only one a host
without the GitHub CLI or Homebrew needs. Download the archive for your platform
from the [latest release](https://github.com/fmueller/orgtop/releases/latest),
extract it, and run `orgtop`.

**Homebrew** (macOS and Linux, amd64 and arm64):

```bash
brew install fmueller/tap/orgtop
```

**GitHub CLI extension** (all six platforms):

```bash
gh extension install fmueller/gh-orgtop
gh orgtop --repo acme/widget
```

`gh orgtop` forwards its arguments unchanged and adds no credential of its own:
it resolves a token by the same precedence a direct `orgtop` launch uses. The
GitHub CLI installs the executable under its own name rather than adding
`orgtop` to `PATH`; the archive and Homebrew routes install the ordinary
`orgtop` command.

Or install with Go:

```bash
go install github.com/fmueller/orgtop/cmd/orgtop@latest
```

A `go install` build reports `dev` for `--version`; only release archives carry
the tag. Build from source with `task build` or `go build ./cmd/orgtop`; see
[CONTRIBUTING.md](CONTRIBUTING.md) for the development setup.

### Verifying a download

Every release publishes `checksums.txt` and a `provenance.intoto.jsonl` bundle
of GitHub build attestations covering all twelve artifacts:

```bash
sha256sum --check --ignore-missing checksums.txt
gh attestation verify orgtop_<version>_<os>_<arch>.tar.gz --repo fmueller/orgtop
```

`fmueller/orgtop` is the only publisher. The `gh-` repository name and the
`gh-extension` topic are GitHub CLI discovery metadata, not an endorsement, and
Homebrew trusts the tap and the formula hash rather than attesting the bytes
itself; the checksum and the attestation above are the verification routes.

## Usage

```bash
orgtop --repo OWNER/REPOSITORY [--repo OWNER/REPOSITORY ...] [--path (PATTERN | OWNER/REPOSITORY:PATTERN) ...] [--no-cache]
orgtop --path OWNER/REPOSITORY:PATTERN [--path OWNER/REPOSITORY:PATTERN ...] [--repo OWNER/REPOSITORY ...] [--no-cache]
orgtop --reset-cache
orgtop --version
```

Repeat `--repo` to select several repositories:

```bash
orgtop --repo acme/backend --repo acme/frontend
```

Every repository is named exactly as `owner/repository`. There is no glob or
organization-wide repository selection: a launch without a selection, with a
malformed identifier, or with glob syntax in a repository name exits before the
terminal UI with usage and a concise cause, and makes no network request.

`--path` narrows a selection to the files an event changed. A bare pattern
filters every `--repo` selection, so the repositories become filtered selections
rather than whole-repository selections:

```bash
orgtop --repo acme/backend --repo acme/frontend --path 'src/**' --path 'docs/**'
```

A qualified `OWNER/REPOSITORY:PATTERN` pattern names its own repository and
needs no `--repo`, and both forms may be mixed:

```bash
orgtop --repo acme/backend --path 'acme/frontend:src/**'
```

A pattern is `/`-separated on every platform, matches case-sensitively, uses `*`
within one segment and `**` as a complete recursive segment, and escapes a
literal `*`, `:`, or `\` with a backslash. Patterns are inclusive only; quote
them so the shell passes them through unchanged.

<!-- docs:path-diagnostics -->
### Invalid path diagnostics

A rejected `--path` value reports one cause. A rejected pattern reports it with
a zero-based UTF-8 byte offset counted from the pattern the diagnostic quotes
rather than from the whole value: in a qualified `OWNER/REPOSITORY:PATTERN`
value the pattern begins at byte 0, and a bare value is its own pattern. The
repository prefix of a qualified value is validated before the pattern, so a
malformed prefix is reported even when the pattern is invalid too; that
diagnostic quotes the prefix and carries no byte offset.

| Value | Diagnostic |
|---|---|
| `--path 'acme/api:'` | `--path: invalid path pattern "" at byte 0: empty pattern` |
| `--path 'acme/api:/src'` | `--path: invalid path pattern "/src" at byte 0: empty segment` |
| `--path 'acme/api:src//api'` | `--path: invalid path pattern "src//api" at byte 4: empty segment` |
| `--path 'acme/api:src/'` | `--path: invalid path pattern "src/" at byte 3: empty segment` |
| `--path '/src'` | `--path: invalid path pattern "/src" at byte 0: empty segment` |
| `--path 'acme/*:src//x'` | `--path: invalid repository identifier "acme/*": repository contains an unsupported character "*"` |

`--no-cache` runs without opening, reading, or writing the enrichment cache.
`--reset-cache` removes OrgTop's cached enrichment state and exits without
resolving a credential, making a request, or starting the terminal UI; it
accepts no other flag.

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

<!-- docs:stream-columns -->
### Stream columns

Stream lists events newest first under a heading row that stays on screen while
the list scrolls. A narrow terminal shortens the headings and the category
names, so both spellings are listed:

| Column | Narrow | Meaning |
|---|---|---|
| `age` | `age` | How long before the last successful refresh the event happened |
| `repository` | `repo` | The repository the event belongs to |
| `category` | `type` | The event category as text: `push`, `pull request`, `review`, `comment`, or `other`; a narrow terminal shortens `pull request` to `pull` and keeps the rest |
| `scopes` | `scopes` | Which selected Scopes the event belongs to, by their `R1`/`P2` tokens: `in` lists the confirmed members, `unresolved` lists the path Scopes whose membership stayed undecided, `~` marks a member proven from an open pull request's current files |
| `actor · description` | `detail` | Who acted, and a concise description of what the event did |

Times are ages, not clock times. A row reading `3d` is the event's
age at the last successful refresh the header reports, so it stays put while a
later failed refresh leaves the header `STALE`.

An event no selected Scope confirmed and no path Scope left undecided keeps no
row. A narrow terminal shortens the Scope column to the counts it cannot spell
in full, so no membership disappears without being counted.

Above the headings Stream states how much activity it is showing: the number of
events it lists and, when the 500-event bound discarded older activity, that
the list stops at that limit. Content too wide for the terminal
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
