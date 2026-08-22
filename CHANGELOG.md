# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Release notes are extracted from the section matching the release tag, so every
tag must have a `## [<version>]` heading here before it is pushed. The release
workflow refuses to publish otherwise.

## [Unreleased]

### Added

- Event domain types and repository scope parsing.
- Command-line configuration and GitHub credential resolution.
- A bounded GitHub activity source with payload normalization.
- Filtered activity snapshots and per-repository aggregates.
- A Bubble Tea application shell with an asynchronous refresh lifecycle.
- Overview now lists every selected repository with its recent event,
  pull-request, and push counts, and says so explicitly when a refresh returns
  no activity at all.
- Stream now lists recent events newest first with their time, repository,
  text-encoded category, actor, and description, and scrolls with the up, down,
  page up, and page down keys.
- The README documents the repeated `--repo` usage, the credential precedence
  and its `gh auth login` fallback, the polling-not-live semantics, the
  controls, and the development checks.

### Fixed

- A `STALE` header now keeps the last successful refresh time when the terminal
  is too narrow to show it beside the Scope summary as well.

[Unreleased]: https://github.com/fmueller/orgtop/commits/main
