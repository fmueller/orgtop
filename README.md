# OrgTop

[![CI](https://github.com/fmueller/orgtop/actions/workflows/ci.yml/badge.svg)](https://github.com/fmueller/orgtop/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/fmueller/orgtop)](https://github.com/fmueller/orgtop/releases/latest)
[![Go Version](https://img.shields.io/github/go-mod/go-version/fmueller/orgtop)](go.mod)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue)](LICENSE)

`top` for your engineering organization: a terminal-native view of what is
happening across the repositories you care about, without opening a browser
dashboard.

<!-- docs:view-selection -->
OrgTop shows recent activity for an explicitly selected set of GitHub
repositories. It reads; it never writes to GitHub, and it stores no credential
of its own.

### Views

OrgTop has three primary views: Overview summarizes each Scope, Stream lists
individual events and opens local event detail, and Rain shows an ambient
activity field with its `Interesting Now` strip.

<!-- docs:overview-counts -->
### Overview counts

Overview reports direct counts from the retained normalized event snapshot for
each repository or path Scope. `N activity` counts confirmed member events;
`U unknown` is separate coverage that keeps that activity count a lower bound.
These are retained snapshot event counts, not rates or fixed-window totals. A
Scope row with zero confirmed activity and zero unknowns says `No activity in retained snapshot`: it is a bounded observation, not a claim that older repository history is empty. A row with unknown evidence says `No confirmed activity · U unknown`, so it is not presented as a confirmed empty Scope. Each refresh fetches at most the newest 100 events per repository and retains the newest 500 unique events globally, so the snapshot does not represent complete history.
The PR-related count is labeled `PR event`/`PR events` (compactly `PR evt`/
`PR evts`): it counts each pull-request event, review, and pull-request comment
separately, even when they refer to the same PR. An unrelated issue comment is
not a PR event, and the count is not a count of distinct, open, or waiting PRs.

`current PR evidence` (compactly `cur PR~`, where `~` is the ASCII qualified-
current-PR marker) is the qualified subset of `activity` whose path membership
was proven from an open PR's current files. At the tightest widths, Overview
uses `PR evts` for PR events and `PR~` for current-PR evidence to keep the
direct counts beside the Scope token. It is evidence about the event's
current-file membership, not an open-PR backlog count or a claim about files at
the event's historical time. Unknown membership remains `U unknown` rather than
being treated as either member or not-member.

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
orgtop (--org ORGANIZATION | --repo 'ORGANIZATION/*') [...] [--repo OWNER/REPOSITORY ...] [--path OWNER/REPOSITORY:PATTERN ...] [--include-archived] [--include-forks] [--no-cache]
orgtop (--org ORGANIZATION | --repo 'ORGANIZATION/*') [...] --repo OWNER/REPOSITORY [--repo OWNER/REPOSITORY ...] --path PATTERN [--path PATTERN ...] [--path OWNER/REPOSITORY:PATTERN ...] [--include-archived] [--include-forks] [--no-cache]
orgtop --reset-cache
orgtop --version
```

Repeat `--repo` to select several repositories:

```bash
orgtop --repo acme/backend --repo acme/frontend
```

Every exact repository is named `owner/repository`. A launch without a selection,
with a malformed identifier, or with unsupported glob syntax in an exact
repository name exits before the terminal UI with usage and a concise cause, and
makes no network request.

<!-- docs:organization-selection -->
### Organization selection

Use `--org ORGANIZATION` or the `--repo 'ORGANIZATION/*'` alias to select an
organization's eligible repositories without naming each one. Exact repository,
qualified path, and organization selections may be mixed:

```bash
orgtop --org acme --repo other/api --path 'other/api:src/**'
```

`--include-archived` and `--include-forks` widen every organization selector;
they do not change exact repository or path Scopes. A selection accepts at most
20 distinct repositories and 100 total Scopes. Exact selections keep capacity
first; organization expansion truncates deterministically when the remaining
capacity is exhausted, reports omitted eligible repositories (and when
applicable that more may remain), and does not present the retained subset as the
whole organization.

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

<!-- docs:enrichment-cache -->
### Enrichment cache

Deciding whether an event changed a selected path needs that event's changed
files. OrgTop reuses that evidence from a local SQLite database rather than
asking GitHub for it again.

The cache lives in the fixed `orgtop` directory beneath the user cache directory
your platform reports (`~/.cache/orgtop` on Linux, `~/Library/Caches/orgtop` on
macOS, `%LocalAppData%\orgtop` on Windows). It holds `enrichment-v1.db` beside
its `enrichment-v1.lock` maintenance lock. There is no path override. On POSIX
systems the directory is created mode `0700` and its files mode `0600`.

It stores only immutable changed-file evidence and the facts that validate it:
no credential, no authorization or response header, no raw GitHub payload, no
rendered screen, and no Scope or event identity. A denied, rate-limited, failed,
or canceled lookup is never stored as evidence of absence.

A record stays usable for 30 days from the moment it was acquired; reuse never
extends that. The database is bounded at 10,000 evidence records, 250,000 stored
paths, and 128 MiB of files on disk. Reaching a bound, or finding expired
records, requests cleanup: a refresh removes at most one bounded batch — invalid
records first, then expired ones, then the least recently used — so cleanup
never pauses the interface while it converges toward its retained targets.

The cache is disposable. `--no-cache` runs one process without opening, reading,
or writing it at all, and `--reset-cache` removes OrgTop's own database and its
sidecars, touching no credential and no unrelated file. Removing it costs
GitHub requests on the next refresh; it never changes which files a Scope
matches. A cache that is missing, busy, unwritable, over its ceiling, or written
by another version is bypassed rather than repaired in place: the refresh reads
from GitHub instead and the header shows `CACHE DEGRADED`, and a database from
another version asks you to update OrgTop or run `--reset-cache`.

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

<!-- docs:github-requests -->
### GitHub requests and rate limits

An authenticated token gets 5000 GitHub REST requests per hour. OrgTop spends
one request per selected repository per refresh, so a selection stays well
inside the budget at the 60-second poll floor; a very large selection is bounded
by that hourly limit rather than by OrgTop.

Changed-file enrichment can spend more. When a path Scope needs evidence an
event does not carry, OrgTop asks GitHub for that entity's changed files, up to
at most 20 changed-file requests per refresh; expanding an organization
selector spends up to five more. Work is deduplicated per entity, so adding
path Scopes over the same repositories costs no extra lookup, and a cache hit
costs none at all.

When the hourly limit is exhausted, the header shows `RATE LIMITED` with the
retry time GitHub instructed, the interface stays responsive, and the last
successful snapshot stays visible. Path membership that could not be decided
stays unknown: it is counted as `U unknown` and shown as `PATH ?` in the header,
and is never guessed into a member or a non-member. `CACHE DEGRADED` marks a
refresh that had to work without the cache, and `TRUNCATED` marks a selection
whose retained snapshot dropped older events at its bound. A narrow header that
cannot fit every badge ends in a `status` count of the ones it held back rather
than dropping them.

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
retained events it lists, the newest-100-per-repository fetch and newest-500
global retention bounds, and, when the global bound discarded older activity,
that the list stops at that limit. Content too wide for the terminal
ends in `…`, marking that row as shortened rather than complete.

<!-- docs:rain-windows -->
### Rain windows

Rain draws one column per selected Scope and keeps an event in the field while
it is inside the selected recency window. `-` selects the next shorter window
and `+` the next longer one; neither wraps at its end. The context line under
the field states the selection as `window 24h`.

| Window | Keeps an event for |
|---|---|
| `15m` | 15 minutes |
| `30m` | 30 minutes |
| `60m` | 60 minutes |
| `6h` | 6 hours |
| `24h` | 24 hours |
| `7d` | 7 days |
| `available` | As long as the last refresh still returned it |

A session defaults to `24h`, so a quiet repository still has an ambient field.
Each finite window drops an event the moment it reaches that age.

`available` is not a longer window but the absence of one: it shows every
event in the bounded snapshot the last refresh returned, whatever its age. The
source fetches at most its newest 100 events per repository and the application
retains at most the newest 500 unique events globally; this is not complete repository history,
and a response page is not treated as proof that older
source activity is absent. A window change and `available` alike filter only
what has already been fetched, so a `24h` or `7d` window does not guarantee
coverage of that whole duration and neither asks GitHub for more.

How old an event looks is separate from how long it is kept. An event past 60
minutes is drawn at the faintest emphasis but stays in the field under `6h`,
`24h`, `7d`, and `available`; only the selected window removes it. A terminal
without distinct intensity is given the same information as text, as
`recency: 1 new · 1 recent · 1 aging · 1 old`.

The separate `Interesting Now` strip samples the last 15 minutes as a
Scope-fair recent-event sample, not an importance ranking. Its fixed 15-minute
window is independent of Rain's selected window and its `-`/`+` controls.

<!-- docs:rain-membership -->
### Rain columns and pause

Rain draws one column per visible Scope, and repository and path Scopes use the
same column model. An event that belongs to several visible Scopes is drawn in
every matching Scope column rather than being assigned to one of them; it stays
one normalized event, and each drawn copy counts against that column's capacity
and the field's global bound. `[` and `]` page through the Scope columns a
narrow terminal cannot show at once, and the context line accounts for the
columns and items a width is holding back.

`p` pauses and resumes Rain motion. A pause freezes movement, ageing, and window
expiry in the field; it does not stop polling, so refreshes continue behind it.
Events that arrive while Rain is paused are queued in a deterministic order
inside the same bounds and are admitted fairly when motion resumes. Ages
elsewhere — the header's freshness, Stream's rows, and the `Interesting Now`
strip — keep advancing while Rain is paused.

### Interesting Now

Rain's `Interesting Now` strip is a bounded Scope-fair recent-event sample from
the last 15 minutes (`0 <= age < 15m`). It is direct recent activity, not an
importance ranking, anomaly signal, or complete-history feed. Its wide title
states `Interesting Now (last 15m)` and accounts for shown, retained-hidden, and
capacity-omitted entries. At narrower widths it uses `I15m:` forms; a collapsed
strip uses `q I15+` when entries are hidden or omitted, and `2` opens Stream where
every retained eligible event remains reachable.

<!-- docs:stream-controls -->
### Controls

Stream uses a visible focus marker so arrow-key and page-key navigation is
observable even when the viewport stays still. `enter detail` opens local event
detail for the focused event; `esc back` returns to that event and its viewport.

| Key | Action |
|---|---|
| `1` | Open Overview |
| `2` | Open Stream |
| `3` | Open Rain |
| `tab` | Cycle through the three views |
| `up` / `down` | In Overview, scroll one row; in Stream, move focus one event (the viewport follows only when focus reaches an edge); in event detail, scroll one line |
| `pgup` / `pgdown` | In Overview, scroll one page; in Stream, move focus by the visible event-row page; in event detail, scroll one page |
| `enter` | In Stream, open detail for the visibly focused event |
| `esc` | In Stream event detail, return to the focused event and its viewport |
| `-` / `+` | Select the shorter or longer Rain window |
| `p` | Pause and resume Rain motion, without pausing polling |
| `[` / `]` | Show the previous or next page of Rain Scope columns |
| `q` | Quit |
| `ctrl+c` | Quit |

At narrow widths, Stream's footer keeps the active detail action beside `q quit`:
the focused list shows `enter detail`, and open event detail shows `esc back`.
Other optional hints yield first; Rain keeps its own strip accounting in the
footer.

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
