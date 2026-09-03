# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Release notes are extracted from the section matching the release tag, so every
tag must have a `## [<version>]` heading here before it is pushed. The release
workflow refuses to publish otherwise.

## [Unreleased]

### Added

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
  does not own rather than deleting it.
- A launch recovers from a structurally corrupt enrichment cache once per
  process by discarding it and creating an empty one, and never serves the rows
  that happened to survive the damage.

### Changed

- Bare `--path` patterns turn their `--repo` selections into filtered path
  Scopes rather than whole-repository Scopes. Repository-only invocations keep
  their v0.1 meaning.

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

[Unreleased]: https://github.com/fmueller/orgtop/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/fmueller/orgtop/releases/tag/v0.1.0
