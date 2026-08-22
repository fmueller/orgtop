# AGENTS.md

Guidance for coding agents working in the OrgTop repository.

## Scope and intent

- This is a Go CLI/TUI repository: `github.com/fmueller/orgtop`.
- The executable entry point is `./cmd/orgtop`; the binary name is `orgtop`.
- Internal application packages belong under `./internal/...` only when they have a current responsibility.
- Product specifications live under `./specs/...`; tracked work lives under `./planning/...`.
- Keep changes small and cohesive. Do not scaffold roadmap architecture without an active requirement.

## Source of truth

- Taskrail is the execution and specification system for product work.
- Before implementing behavior, run `taskrail status` and inspect the active spec named in `planning/STATE.md`.
- Treat the active versioned spec and the selected task's acceptance criteria as normative.
- `Taskfile.yml` defines standard local and CI commands.
- `mise.toml` pins the developer and CI toolchain.
- `.github/workflows/ci.yml` defines required CI validation for code changes.
- `.github/workflows/planning.yml` is the fast lane for planning, spec, doc, and
  skill changes; its `paths:` set must stay an exact mirror of the `paths-ignore`
  in `ci.yml`, and `TestWorkflowPathLanesAreExactMirrors` fails when it does not.
- `.github/workflows/release.yml` publishes tags; `CHANGELOG.md` is the source of
  release notes and a tag without a matching `## [<version>]` section is refused.
- `internal/toolchain` holds no production code: it guards the repository's own
  CI, release, and toolchain configuration.
- `README.md` is the repository-level product and contributor introduction.

## Taskrail lifecycle

- Run `taskrail validate` before selecting or changing tracked work.
- Use `taskrail next`, `taskrail start`, `taskrail complete`, and `taskrail verify` for lifecycle transitions.
- Inspect `taskrail <command> --help` rather than guessing flags.
- Do not manually modify Taskrail-managed execution state, including `planning/STATE.md` frontmatter or task status fields.
- Create and mutate tracked tasks through Taskrail's supported commands and installed skills.
- Inspect the diff and run `taskrail validate` after every Taskrail state-changing operation.
- Do not start work on an ineligible task or implement future roadmap phases unless the active spec requires them.

## Architecture boundaries

- Keep GitHub API payload and transport types in the source adapter. They must not become OrgTop domain or TUI models.
- Normalize source data before filtering, aggregation, application state, or rendering.
- Keep domain calculations out of Bubble Tea rendering functions.
- Keep source, domain, application, and TUI responsibilities distinct without inventing unused abstraction layers.
- Prefer the standard library for flags while OrgTop remains a single command. Do not add Cobra without a real subcommand hierarchy.
- Do not add Bubbles unless a concrete component is needed. Do not add Harmonica unless an active future spec requires it.

## Toolchain and commands

- Go is pinned in `go.mod` and `mise.toml`.
- Bubble Tea v2 and Lip Gloss v2 are the intended TUI framework and styling layer.
- Direct Go commands are canonical; Task targets provide a consistent developer and CI interface.
- Build: `task build` (`go build ./cmd/orgtop`).
- Test: `task test` (`go test ./...`).
- Cross-compile smoke: `task build:cross` (every platform `.goreleaser.yml` publishes).
- Startup smoke: `task run:smoke`.
- Format: `task fmt` (`gofmt -w .`).
- Format check: `task fmt:check`.
- Lint: `task lint` (`golangci-lint run ./...`).
- Vet/static analysis: `task vet` (`go vet ./...`).
- Dependency licenses: `task licenses:check`.
- Full local gate: `task check`; it mirrors the CI `checks` job step for step, and
  `TestLocalCheckMirrorsTheCIChecksJob` fails if the two drift apart.
- Taskrail validation: `task workflow:validate` (`taskrail validate`).
- Advisory spec coverage: `task workflow:verify:spec` (`taskrail coverage`).
- Release configuration: `task release:check`; local snapshot: `task release:dry`.
- Install opt-in hooks: `task hooks-install`.
- Differential mutation tests: `task test:mutate` (override with `BASE=<ref>`).
- Full mutation gate: `task test:mutate:gate`; run it deliberately, while CI runs
  it weekly and on manual dispatch.
- The flat legacy names (`format`, `format-check`, `taskrail-validate`,
  `release-check`) survive as aliases; prefer the `group:verb` names.

## Go conventions

- Run `gofmt` on changed Go files.
- Keep functions focused and prefer early returns on error paths.
- Return errors rather than panicking; wrap causes with context using `%w`.
- Error strings should be lowercase and should not end in punctuation.
- Pass `context.Context` first for cancelable operations and bind network/process work to it.
- Prefer typed structs over `map[string]any` for product flows.
- Package names should be short, lowercase, and noun-like.
- Add dependencies only when they solve a current requirement.

## Testing

- Use the standard `testing` package and table-driven tests where useful.
- Keep ordinary unit tests deterministic and independent of live GitHub or user credentials.
- Use fixtures or fakes for GitHub payloads, failures, pagination, and cancellation.
- Test domain behavior and important TUI states separately.
- Add tests with behavior changes rather than postponing all testing to an integration task.
- Run targeted tests while iterating and `task check` before handing off a completed change.
- Use differential mutation testing for logic-heavy changes; do not put the full
  gate on routine pull-request CI.

## Changes and commits

- Prefer small, cohesive changes that preserve clear package ownership.
- Coding agents must run `task hooks-install` before creating their first commit
  in a worktree; do not assume the local `commit-msg` hook is already installed.
- Use Conventional Commits with imperative subjects.
- Include a descriptive body after the subject, wrap body lines at 72 characters,
  and suffix tracked-task subjects with the short key, for example `(T-001)`.
- Keep tracked tasks focused and include objective verification evidence.
- Do not mix unrelated refactors or formatting churn into feature changes.
- Do not rewrite shared history or bypass CI-equivalent checks.
- Update public documentation when user-visible behavior or setup changes.
