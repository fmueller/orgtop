# Contributing to OrgTop

Humans and AI agents both contribute here. This is the fast path plus the
human-facing rules. For depth:

- **Coding agents / AI tools:** [`AGENTS.md`](AGENTS.md) is authoritative; read it first.
- **Changelog policy:** [`docs/changelog.md`](docs/changelog.md).
- **Product scope:** the versioned specs under [`specs/`](specs/).

## AI-assisted contributions

AI-generated and AI-assisted pull requests are welcome. Two rules apply:

1. **You own the diff.** Whoever opens the pull request is accountable for every
   line, whether written by a human or a tool.
2. **No bot attribution.** Do not add `Co-Authored-By: <bot>`, `Assisted-By:`,
   `Generated with ...`, or 🤖 trailers. The `commit-msg` hook rejects them. The
   AI is a tool, not a co-author.

The same quality gate applies regardless of how the code was produced.

## Setup

```sh
git clone https://github.com/fmueller/orgtop.git
cd orgtop
mise run setup    # pinned toolchain, builds bin/orgtop, installs opt-in git hooks
```

`mise.toml` pins Go, Task, golangci-lint, GoReleaser, Lefthook, go-licenses, and
Taskrail. Direct `go` and `task` commands work without mise if you provide the
tools another way. `task hooks-install` installs the hooks on their own.

## Before you open a pull request

- Run `task check`. It is the full local gate and mirrors the CI `checks` job
  step for step: `fmt:check`, `vet`, `lint`, `test`, `test:changelog`, `build`,
  `run:smoke`, `licenses:check`, `workflow:validate`, `workflow:verify:spec`,
  and `release:check`.
- Run `taskrail validate` if you touched `planning/`, `specs/`, or `.taskrail/`.
  `task workflow:validate` does this for you.
- Use a Conventional Commit subject. Types: `feat fix refactor docs test chore
  build perf ci`. End tracked-task subjects with only the short key, for example
  `feat: report the release version from the binary (T-028)`. Never a task
  prefix or the full slugged identifier; non-tracked commits may omit the
  reference.
- After the subject and a blank line, include a concise body explaining the
  commit's intent, context, and non-obvious decisions rather than restating the
  diff. Wrap body lines at 72 characters. The `commit-msg` hook requires this
  body and line limit for ordinary commits.
- Keep a tracked task's implementation, tests, and required planning metadata in
  one commit.
- Update `CHANGELOG.md` under `## [Unreleased]` for user-visible changes only.
  Follow the [changelog policy](docs/changelog.md).
- Update `README.md` for installation, usage, authentication, or view-behavior
  changes.

CI remains authoritative and selects validation by the paths changed.

## Continuous integration

Two lanes split the work by what changed:

- `CI` runs formatting, vet, lint, licenses, Taskrail validation, and
  release-config checks, then a build/test matrix across Linux (x86-64 and
  arm64), Windows, and macOS, plus a cross-compile smoke over every platform the
  release publishes. Pull requests run the matrix on Linux only for fast
  feedback; pushes to `main` and manual dispatch run all of it.
- `Planning checks` is the fast lane for planning, spec, doc, and agent-skill
  changes. It keeps the Taskrail and skill-mirror gates without starting the
  matrix. Its trigger paths are the exact mirror of the paths `CI` ignores, and
  `TestWorkflowPathLanesAreExactMirrors` fails when they drift.

`task test:mutate` runs differential [Gremlins](https://gremlins.dev/) mutation
testing against `main` (`BASE=<ref>` overrides it). The full gate runs weekly
and on manual dispatch in its own workflow, never on a pull request.

The two lanes deliberately run different mutator sets. The differential run
keeps the five mutators gremlins enables by default, because it is the lane
repeated per change. The weekly gate adds the six gremlins ships disabled
(`--invert-logical`, `--invert-loopctrl`, `--invert-assignments`,
`--invert-bitwise`, `--invert-bwassign`, `--remove-self-assignments`), which
finds inverted guards and loop-control mistakes the default set cannot reach.
`TestMutationTiersSplitTheMutatorSet` fails if either lane drifts into the
other's set.

## Tracked work

Versioned product contracts live in `specs/`, and implementation tasks live in
`planning/tasks/`. Before selecting work:

```sh
taskrail validate
taskrail status
taskrail next
```

Use the Taskrail lifecycle to start, complete, and verify tracked work. Never
hand-edit `planning/STATE.md` execution fields or task status fields; route
changes through the CLI and commit the regenerated files in the *same* commit as
the change that produced them. Use the Taskrail version pinned in `mise.toml`.

## Releases

`CHANGELOG.md` is the source of release notes. Add a `## [<version>]` section
before tagging: the release workflow refuses to publish a tag whose section is
missing or empty. Pushing a `v*` tag builds and publishes through GoReleaser; a
manual dispatch builds a local snapshot and publishes nothing.

## Scope

Keep pull requests focused on one logical outcome. Ask first before changing
spec contracts, tracked-work schemas, or CI gates; adding a runtime dependency;
or starting a broad refactor. Specs are normative and should be changed only
when the work explicitly calls for it. OrgTop stays a terminal-native read-only
activity view: not a GitHub client, not a writer, not a daemon.

## License

By contributing, you agree that your contributions are licensed under
Apache-2.0 (see [LICENSE](LICENSE)).
