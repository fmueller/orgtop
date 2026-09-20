# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Release notes are extracted from the section matching the release tag, so every
tag must have a `## [<version>]` heading here before it is pushed. The release
workflow refuses to publish otherwise.

## [Unreleased]

### Added

- One tag now publishes through three channels. Alongside the release archives,
  a GitHub CLI extension release in `fmueller/gh-orgtop` carries a raw
  `gh-orgtop-<os>-<arch>` executable for each of the six platform targets, and a
  Homebrew formula in `fmueller/homebrew-tap` installs the macOS and Linux
  builds as `brew install fmueller/tap/orgtop`. Every channel redistributes the
  byte-identical executable one build produced: the extension assets are that
  build copied and renamed, and the formula downloads the release archives and
  pins their checksums. `gh orgtop` forwards its arguments unchanged and adds no
  credential path of its own, and the archives remain a complete installation
  path on a host with neither tool.
- Releases publish `checksums.txt` covering all twelve artifacts and a
  `provenance.intoto.jsonl` bundle of GitHub build attestations, plus a
  `distribution-complete.json` manifest binding every published digest to the
  source tag, commit, and workflow. `README.md` documents the checksum and
  attestation verification routes.
- The release workflow stages every channel before anything becomes public and
  fails closed. A missing, extra, renamed, rebuilt, or digest-divergent artifact
  stops the release before publication; a channel that fails leaves an
  explicitly partial release that is retried or withdrawn rather than reported
  as complete. `docs/distribution-ledger.jsonl` records each staged, completed,
  and withdrawn version, and a withdrawal keeps a durable notice under
  `docs/withdrawals/`.
- `--path` selects path Scopes. A bare `PATTERN` filters every `--repo`
  selection, a qualified `OWNER/REPOSITORY:PATTERN` stands on its own, and both
  forms may be repeated and mixed. Equivalent Scopes are deduplicated and keep
  their first requested spelling.
- `--org ORGANIZATION`, and its `--repo 'ORGANIZATION/*'` alias, select every
  eligible repository of an organization. A launch expands its selectors before
  it polls anything, re-expands every 15 minutes, and polls one immutable
  selection per refresh. `--include-archived` and `--include-forks` widen that
  eligibility. A failed re-expansion keeps the last successful selection and
  marks it stale rather than narrowing or emptying it; an initial expansion
  failure polls no subset, and a successful expansion with no eligible
  repository is a current empty selection.
- The shared header states every secondary condition of a refresh beside the
  primary source state, in one fixed priority: `RATE LIMITED <retry>`,
  `SELECTION STALE`, `PATH ?U` for unresolved path evidence, `CURRENT PR Q` for
  the qualified current-PR members, `CACHE DEGRADED`, and `TRUNCATED`. A width
  too narrow for all of them spends its last badge slot on `+N status` rather
  than dropping a condition silently, and every badge is read from prepared
  state instead of the sanitized failure text, so a rate limit is stated only
  when a refresh actually reported one.
- The shared header discloses an organization selection as
  `selection: R repos · S scopes · X exact · G expanded`, appends the eligible
  omission count and a remaining-page warning when a bounded expansion could
  not admit everything, and marks a selection that a failed re-expansion left
  behind as `SELECTION STALE` beside the primary state.
- `--no-cache` runs a launch without any enrichment-cache operation, and
  `--reset-cache` is a standalone administrative action that removes the
  disposable enrichment cache and exits without resolving a credential, making
  a request, or starting the terminal UI. The removal renames the database
  before deleting its write-ahead sidecars, so no launch ever opens a fresh
  database beside stale write-ahead state, and it preserves a database OrgTop
  does not own rather than deleting it. Every other launch opens the
  enrichment cache at its default location and reuses fresh complete
  changed-file evidence instead of re-requesting it; a cache that cannot be
  opened keeps the launch running and reports it as degraded rather than
  failing the start.
- A launch recovers from a structurally corrupt enrichment cache once per
  process by discarding it and creating an empty one, and never serves the rows
  that happened to survive the damage.
- On Windows the enrichment cache is now used only when its directory and files
  belong to the account running OrgTop and no other account may write to them.
  A cache another user owns or may write to is reported as degraded and left
  untouched, matching the ownership rule already enforced on macOS and Linux.

### Changed

- Overview, Stream, and Rain now qualify successful empty states and activity
  counts by the retained snapshot. Coverage discloses the bounded newest-100
  per-repository fetch and newest-500 global retention, without presenting a
  quiet retained row or a Rain duration as complete history.
- Rain offers the windows `15m`, `30m`, `60m`, `6h`, `24h`, `7d`, and
  `available`, and a session now starts at `24h` instead of `60m`, so a quiet
  repository keeps an ambient field. `-` and `+` step through the presets and
  stop at either end. `available` keeps every event of the snapshot the last
  refresh returned, whatever its age: that is the newest 100 events per
  repository GitHub served, not complete repository history, and an event
  leaves the field once a later successful refresh no longer returns it.
- Rain visual recency and Rain removal are now separate. An event past 60
  minutes is drawn as `old` and stays in the field under `6h`, `24h`, `7d`, and
  `available`; only the selected window removes it. The no-color recency
  accounting states the old items too, as
  `recency: N new · R recent · A aging · O old` or `age N/R/A/O`.
- Rain's `Interesting Now` strip now discloses its independent fixed last-15-minute
  Scope-fair recent-event sample, rather than leaving its shown/hidden/omitted
  accounting or empty state open to an importance-ranking interpretation. Narrow
  layouts retain the `I15m:` forms and the `q I15+` overflow hint.
- `--help` now lists the Rain windows beneath the flags, with the preset a
  session starts at and what `available` covers, so the bound is readable
  without opening the README.
- Rain now reports its visible Scope range and disjoint hidden Scope/item
  counts in the shared header instead of repeating them in the body context.
- Overview and Stream now report the visible row range when terminal height
  hides content. Stream navigation tracks a focused event and scrolls only when
  that focus crosses the viewport, preserving it across resize and refresh.
- Stream now marks the focused event as arrow keys move within a stationary
  viewport, with an accessible UTF-8/ASCII indicator, and documents the focus,
  detail, and return controls.
- At narrow widths, Stream keeps `enter detail` or `esc back` beside `q quit`
  in the active footer before optional navigation hints yield; Rain retains its
  strip accounting.
- Bare `--path` patterns turn their `--repo` selections into filtered path
  Scopes rather than whole-repository Scopes. Repository-only invocations keep
  their v0.1 meaning.
- Stream rows name the Scopes an event belongs to: `in` lists the confirmed
  members by their `R1`/`P2` tokens, `unresolved` lists the path Scopes whose
  membership stayed undecided, and `~` marks a member proven from an open pull
  request's current files. An event no Scope confirmed and none left undecided
  no longer takes a row, and a narrow terminal counts the Scopes it cannot
  spell rather than dropping them.
- Stream names an event category from the shared category vocabulary every
  event view will render through. A narrow Stream now spells `pull`, `review`,
  `comment`, and `other` instead of the abbreviations `pr`, `rev`, `com`, and
  `oth`; the wide spellings are unchanged. A category a release does not know
  normalizes to `other` rather than reaching a row unnamed.
- Overview now labels its direct pull-request-related count `PR events` (compact
  `PR evts`), counting each pull-request event, review, and pull-request comment
  separately rather than implying distinct, open, or waiting pull requests.
  `current PR evidence` (compact `cur PR~`, with `~` as the ASCII evidence
  marker; dense `PR evts`/`PR~` at the tightest widths) identifies the subset
  of member events proven from an open PR's current files, while unknown path
  coverage remains separate.

### Fixed

- Coalesced one-commit push enrichment now checks each event's own `before` SHA,
  including cached evidence, so events sharing a head never borrow another
  event's changed-file proof.

## [0.1.0] - 2026-08-25

### Added

- Event domain types and repository scope parsing.
- Command-line configuration and GitHub credential resolution.
- A bounded GitHub activity source with payload normalization.
- Filtered activity snapshots and per-repository aggregates.
- A Bubble Tea application shell with an asynchronous refresh lifecycle.
- Overview now lists every selected repository with its recent event,
  pull-request, and push counts, and says so explicitly when a refresh returns
  no activity at all.
- Stream now lists recent events newest first with their age at the last
  successful refresh, repository, text-encoded category, actor, and description.
- Overview and Stream both scroll with the up, down, page up, and page down
  keys, each keeping its own position across view switches.
- The README documents the repeated `--repo` usage, the credential precedence
  and its `gh auth login` fallback, the polling-not-live semantics, the
  controls, and the development checks.
- `--version` and its `-v` short form print the release version and exit zero,
  before any credential resolution, subprocess, or terminal UI, and without
  needing a `--repo` selection. The single `orgtop <version>` line goes to
  stdout while usage and startup failures stay on stderr, so a caller can read
  one without the other. Release builds stamp the version onto the binary; a
  build without that stamp reports `dev`.
- Stream names its columns. A heading row sits directly under the shared header
  and stays there at every scroll position, so the age, repository, category, and
  actor/description columns remain identified once the events have scrolled. The
  headings give way when the terminal is too short to hold both them and an event
  row, and the explicit loading, error, and no-recent-activity states keep none.
- Stream states how much activity it is showing above its column headings: the
  number of events the current snapshot holds, and, when the 500-event bound
  discarded older activity, that the list stops at that limit. The disclosure is
  the first line to give way on a short terminal.

### Changed

- Stream and Overview mark content the terminal is too narrow to hold with a
  trailing `…`, so a shortened repository name or description is no longer read
  as the whole value. The mark is paid for out of the same width.
- Event descriptions no longer restate the entity class the Stream category
  column already names: a pull-request row reads `opened #42` rather than
  `opened pull request #42`. Descriptions without an entity reference still name
  the entity, and an issue-only comment keeps its noun because its category is
  `other`.

### Fixed

- A `STALE` header now keeps the last successful refresh time when the terminal
  is too narrow to show it beside the Scope summary as well.

## [0.0.5] - 2026-09-20

### Added

- Not a product release. This version exists only to rehearse the one release-
  path behavior v0.0.3 and v0.0.4 could not prove: a retry of an already
  completed publication that resumes and reconciles rather than failing closed.
  Each earlier attempt exposed a defect on that path and pinned a commit that
  predated its fix. It is withdrawn as soon as the rehearsal is verified, and
  carries no OrgTop behavior of its own; install v0.1.0 or later.

## [0.0.4] - 2026-09-20

### Added

- Not a product release. This version exists only to rehearse the one release-
  path behavior v0.0.3 could not prove: a retry of an already completed
  publication that resumes and reconciles rather than failing closed. It is
  withdrawn as soon as that rehearsal is verified, and carries no OrgTop
  behavior of its own; install v0.1.0 or later.

## [0.0.3] - 2026-09-20

### Added

- Not a product release. This version exists only to rehearse the multi-channel
  release path through a completed publication: the publication transitions,
  the completion manifests, the completed ledger event, final reconciliation,
  and a retry that resumes and reconciles. It is withdrawn as soon as that
  rehearsal is verified, and carries no OrgTop behavior of its own; install
  v0.1.0 or later.

## [0.0.2] - 2026-09-18

### Added

- Not a product release. This version exists only to rehearse the retry path of
  the multi-channel release: it is built, reconciled, and removed without ever
  publishing an asset or recording a ledger event. It carries no OrgTop
  behavior of its own; install v0.1.0 or later.

## [0.0.1] - 2026-09-17

### Added

- Not a product release. This version exists only to rehearse the multi-channel
  release path end to end against the real distribution App, and is withdrawn
  as soon as that rehearsal is verified. It carries no OrgTop behavior of its
  own; install v0.1.0 or later.

[Unreleased]: https://github.com/fmueller/orgtop/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/fmueller/orgtop/releases/tag/v0.1.0
[0.0.5]: https://github.com/fmueller/orgtop/releases/tag/v0.0.5
[0.0.4]: https://github.com/fmueller/orgtop/releases/tag/v0.0.4
[0.0.3]: https://github.com/fmueller/orgtop/releases/tag/v0.0.3
[0.0.2]: https://github.com/fmueller/orgtop/releases/tag/v0.0.2
[0.0.1]: https://github.com/fmueller/orgtop/releases/tag/v0.0.1
