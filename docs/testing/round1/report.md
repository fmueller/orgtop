# Adversarial executable testing — round 1

Date: 2026-10-04. Verdict: **fail, three new confirmed defects; no fixes**.
Pinned spec: `specs/v0.2.0.md`. Tested source:
[`d1dabc3`](https://github.com/fmueller/orgtop/commit/d1dabc381adeec81b8b08da24fea1d4ebabbbf0d).
T-106 remains todo: this orb test is not its maintainer live-host/channel acceptance.

## Environment and method

- Linux amd64 orb, Go 1.26.7 through `mise exec --`; final executable built with
  `mise exec -- env CGO_ENABLED=0 go build -o /tmp/orgtop-e2e ./cmd/orgtop`.
  An initial diagnostic run used the default CGO setting; the final matrix was
  repeated against the pure-Go build used by release builds.
- Unmodified executable, real tmux PTY, real CLI parsing, authentication resolution,
  HTTPS transport, normalization, enrichment, SQLite persistence, scheduling, and
  Bubble Tea rendering. No injected application refresh/model seams.
- A standard-library Python CONNECT proxy listened only on loopback, accepted
  only `api.github.com:443`, terminated TLS with a disposable locally trusted
  certificate, and returned bounded GitHub-shaped JSON. The process received a
  clean environment, synthetic GH_TOKEN, local HTTPS_PROXY/SSL_CERT_FILE, and
  disposable HOME/XDG_CACHE_HOME. No real token or GitHub account was used.
- Request logs retain path, media/API headers and a credential-match boolean,
  never credentials or authorization-header values. A commit response contained
  one parent and one modified `src/main.go` file; the three-event fixture used a
  push, unsupported issue activity, and a direct review comment on `src/lib.go`.
- Captures are actual `tmux capture-pane` text. Review images rasterize that text
  using ImageMagick/DejaVu Sans Mono; they are not photographs or proof of color
  capability. Rain at 120x30 and 40x12 and the hostile Stream image were inspected.
  Bidi behavior is confirmed from captured code points, not inferred from images.

## Executed matrix

| Process flow | Observed result |
| --- | --- |
| Missing selection, bare path with organization only, `../x`, reset plus repository | Usage exit 2 as expected |
| Help before unknown flag | Help, exit 0 |
| Unknown flag before help; version with unknown flag or missing value | **T-119**, exit 2 instead of informational exit 0 |
| Boolean `=true` and reset with `--no-cache=false` | **T-120**, invalid syntax accepted; populated disposable DB deleted |
| Cold mixed repository/path selection | Repository 3 activity; path 2 activity plus 1 unknown |
| Warm second process | No additional commit request: cold 1, cumulative warm 1 |
| `--no-cache` with fresh cache root | Equivalent membership; cache directory not created |
| Enrichment 403 / mismatching commit parent | Repository stays 3; path 1 confirmed, 2 unknown; no guessed membership |
| Source 403 / 429 / malformed JSON shape | Interactive error; 429 displays RATE LIMITED; q exits |
| Successful empty source | Bounded retained-snapshot empty wording, not error |
| 100 events | 100 retained; Stream range 1–26/100; page-down then detail reaches event 27 |
| Organization expansion | One valid organization-owned listing becomes one polled repository; expanded provenance displayed |
| Non-directory cache root | CACHE DEGRADED and equivalent membership, not failed startup |
| Future schema (`user_version=99`) | CACHE DEGRADED; version preserved; valid reset removes it; next launch rebuilds |
| Overview → Stream → detail → Escape → Rain | Navigation, separate detail, pause, and quit exercised |
| Rain repeated resize | 120x30 → 80x24 → 40x12 → 20x6 → 8x3 → 1x1 → 120x30; no crash, restored view and quit |
| C locale plus NO_COLOR | ASCII Rain category glyphs; `-` moves 24h to 15m |
| Interesting Now | Three recent entries, overlap disclosure, 15m window and hidden/omitted accounting visible |
| Initial source failure → success | Waited 61 seconds; actual scheduled retry publishes 3/2 activity without injected messages |
| Pending 8-second source response | q terminates within the 2-second test bound |
| Valid branch containing U+202E | **T-121**, raw control in Stream; detail uses visible `\u{202E}` |

`results.json` contains assertion outcomes, including the expected red assertions
for the two CLI findings. Scenario rows named “quit” assert termination only;
their semantic error/coverage states were separately inspected in captured text.
`requests.json` records actual HTTP requests. These are process E2E results, not
claims of complete spec coverage or the absence of other defects.

## Confirmed findings and reproduction

- **T-119:** `orgtop --bad --help`, `orgtop --version --reset-cache --bad`, and
  `orgtop --version --repo` exit 2. RG-001 requires informational success despite
  otherwise-invalid arguments. `cli.json` retains exact arguments and output.
- **T-120:** `orgtop --reset-cache=true` succeeds, and
  `orgtop --reset-cache --no-cache=false` actually deletes a populated cache.
  RG-001 permits bare booleans only and requires validation before cache work.
  `invalid-reset.json` records a one-evidence-row database hash before deletion,
  exit 0, the success message, and absence afterward. Reproduce only in a
  disposable HOME/XDG_CACHE_HOME, after one cold successful path-enrichment run.
- **T-121:** serve `hostile-event.json` for
  `/repos/acme/api/events?per_page=100`, and `commit.json` for
  `/repos/acme/api/commits/2222222222222222222222222222222222222222?per_page=100`.
  Launch `orgtop --repo acme/api --path acme/api:src --no-cache` at 120x30.
  Press 2, capture, then Enter and capture. Stream contains raw U+202E; detail
  contains ASCII `\u{202E}`. `bidi-check.json` checks both and validates the Git
  ref. The reduced fixture omits the unrelated issue/comment rows in the full
  recorded run; its timestamp need not be fresh for Stream. HTTPS responses use
  JSON content type, correct content length, and a 200 status.

Each task records acceptance criteria, regression expectations, and a genuine
RG-001/RG-012 spec heading. No production changes were made.

## Excluded duplicates, limitations, and checks

- T-118 already owns extension usage naming; no duplicate filed. T-078 already
  owns Windows cache security validation. T-085/T-086 are completed Scope-label
  and detail work, not an open task for the Stream-summary leak.
- Initial fixture errors were excluded: missing organization owner type,
  Escape and 3 sent together becoming Alt+3, and an incorrect hypothesis that
  `[x]` was invalid (RG-002 makes brackets literal). Corrected runs passed.
- A visual reviewer suspected right-edge clipping of a Rain strip row. The raw
  row ends with its complete age, within the 120-cell budget; this was not filed.
- No live GitHub credential/access semantics, real human terminal/font matrix,
  release archive installation, gh extension installation, Homebrew installation,
  macOS, Windows, or arm64 execution. No maintainer sign-off. Non-directory cache
  failure is not a substitute for real-host ACL/unwritable-directory testing.
- No full stale-success→failure→recovery cycle, organization pagination/capacity
  exhaustion, maximum 50-Scope composition, or every combined failure was executed
  here. These remain untested, not passing assumptions.
- Supplemental existing tests: `mise exec -- go test ./cmd/orgtop ./internal/cli
  ./internal/tui ./internal/cache` passed. These injected/unit tests are separate
  from the executable matrix. Planning checks use `task workflow:validate` and
  `task workflow:verify:spec`; Taskrail was validated before writes and after
  every task creation. The tool reports dev-version/installed-skill drift as a
  warning; no skills or unrelated state were rewritten to silence it.

## Selector handoff

`mise exec -- taskrail next --json` still picks high-priority T-106. Candidates
then include T-119, T-120, T-121 (medium) and T-118 (low). `taskrail next --help`
offers only `--include-off-spec` and `--json`; there is no task-ID allowlist or
priority override. The installed autonomous-backlog skill unconditionally uses
that selector. Therefore it cannot currently consume only these follow-ups.

Use explicit `autonomous-task <full-task-id>` / supported `taskrail start <id>`
for T-119, T-120, and T-121, or obtain a separately authorized workflow selector
enhancement. Do not block, reprioritize, complete, or hand-edit unrelated tasks
to force selection. T-106 and T-118 were not mutated.
