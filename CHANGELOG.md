# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Release notes are extracted from the section matching the release tag, so every
tag must have a `## [<version>]` heading here before it is pushed. The release
workflow refuses to publish otherwise.

## [Unreleased]

## [0.2.0]

### Added

- `--path` selects activity by changed files. Repeat or mix bare patterns with
  `--repo` selections and standalone `OWNER/REPOSITORY:PATTERN` selections;
  equivalent Scopes are deduplicated.
- `--org ORGANIZATION` and `--repo 'ORGANIZATION/*'` expand eligible repositories,
  with `--include-archived` and `--include-forks` options. Selection counts and
  omissions stay visible; a failed 15-minute re-expansion preserves the last
  selection and marks it `SELECTION STALE`.
- A bounded local SQLite cache reuses complete changed-file evidence for up to
  30 days. `--no-cache` bypasses it; standalone `--reset-cache` removes OrgTop's
  cache without resolving credentials, contacting GitHub, or launching the UI.
- `CACHE DEGRADED` keeps the application usable when its cache is unavailable.
  Corrupt caches are rebuilt once per launch; incompatible or unsafe caches
  are left untouched, including caches another account can write on Windows.
- The shared header reports rate limits, stale selections, unknown path
  membership, qualified current-PR evidence, cache degradation, and truncation.
  Narrow layouts count conditions they cannot display instead of hiding them.
- Homebrew (`brew install fmueller/tap/orgtop`) and the GitHub CLI extension
  (`gh extension install fmueller/gh-orgtop`) redistribute the same executable
  as the standalone archives, with unchanged arguments and credential handling.
- Releases include checksums, GitHub build attestations, and a completion
  manifest covering all six platforms. Cross-channel publication checks byte
  parity before going public and records partial, completed, and withdrawn
  versions rather than treating a partial publication as success.

### Changed

- Bare `--path` patterns filter their `--repo` selections rather than adding
  whole-repository Scopes. Repository-only invocations retain their v0.1 meaning.
- Overview, Stream, and Rain describe counts and empty states as observations
  of a retained snapshot: at most the newest 100 events per repository and 500
  unique events globally, not complete history.
- Overview labels pull-request-related activity `PR events`, not a count of
  distinct or open PRs. `current PR evidence` identifies the subset proven from
  an open PR's current files; unknown path membership remains separate.
- Stream shows confirmed and unresolved Scope membership, including `~` for
  current-PR evidence. Narrow layouts retain membership counts and readable
  category names rather than silently dropping context.
- Overview and Stream show visible row ranges. Stream has a visible UTF-8/ASCII
  focus marker and preserves focus across resize and refresh; narrow footers
  retain the active detail/back action beside quit.
- Rain starts at `24h` instead of `60m`. `-` and `+` select `15m`, `30m`, `60m`,
  `6h`, `24h`, `7d`, or `available`; the last option keeps every event still in
  the fetched snapshot, not complete repository history. Help lists the windows.
- Rain draws older events at `old` emphasis until the selected window removes
  them, with text recency counts when colour cannot distinguish age. The header
  reports visible columns and hidden Scope/item counts.
- Interesting Now explicitly describes its independent last-15-minute,
  Scope-fair recent-event sample, not an importance ranking. Shown, hidden, and
  omitted counts remain available in narrow and collapsed layouts.

### Fixed

- `gh orgtop --help` and invalid-flag usage show `gh orgtop` instead of the
  extension executable's filename. Direct `orgtop` usage is unchanged.
- Automatic refresh respects both source and enrichment retry deadlines,
  including the latest concurrent rate-limit response. Current activity and
  unknown membership stay visible until a permitted retry can recover them.
- Commit and organization pagination accepts valid quoted and escaped link
  attributes, but rejects malformed, ambiguous, nonadvancing, or duplicate-key
  links rather than confirming partial path evidence or organization selections.
- Missing or null changed-file arrays leave path membership unknown instead of
  caching false absence. Explicit empty arrays still confirm no matching files.
- Push events sharing a head check their own `before` SHA, including cached
  evidence, so one event cannot borrow another event's changed-file proof.
- Stream and Interesting Now render control and bidi characters as visible
  escapes before measuring text, consistently with event detail.
- Boolean assignments such as `--reset-cache=true` and `--no-cache=false` fail
  with usage exit 2 before cache removal or launch work. Bare help/version
  requests take precedence over invalid arguments without accessing credentials
  or the cache; informational-looking selection values remain values.
- Release retries remove the tap staging branch only when it carries exactly
  the formula already published, and ledger writes reject conflicting default-
  branch movement rather than persisting contradictory publication records.

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

[Unreleased]: https://github.com/fmueller/orgtop/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/fmueller/orgtop/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/fmueller/orgtop/releases/tag/v0.1.0
[0.0.5]: https://github.com/fmueller/orgtop/releases/tag/v0.0.5
[0.0.4]: https://github.com/fmueller/orgtop/releases/tag/v0.0.4
[0.0.3]: https://github.com/fmueller/orgtop/releases/tag/v0.0.3
[0.0.2]: https://github.com/fmueller/orgtop/releases/tag/v0.0.2
[0.0.1]: https://github.com/fmueller/orgtop/releases/tag/v0.0.1
