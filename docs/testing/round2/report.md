# Adversarial executable testing — round 2

Date: 2026-10-04 UTC. Pinned spec: `specs/v0.2.0.md`. Baseline:
[`b42b880`](https://github.com/fmueller/orgtop/commit/b42b880952a906bff73478fe219677af5d0cb873),
verified clean and equal to freshly fetched `origin/main` before testing.
Verdict: **one new confirmed defect, T-122; no product fixes implemented**.
T-119/T-120/T-121 regressions pass. T-106 remains untouched and todo.

## Real process method and reproducibility

- Linux amd64 orb, pinned Go 1.26.7 via mise, pure-Go build:
  `mise exec -- env CGO_ENABLED=0 go build -o /tmp/orgtop-round2 ./cmd/orgtop`.
- Unmodified executable runs in actual tmux PTYs. Python's loopback-only HTTPS
  CONNECT fixture accepts only `api.github.com:443`; disposable OpenSSL certificate
  and synthetic GH_TOKEN, isolated HOME/XDG_CACHE_HOME, trusted SSL_CERT_FILE and
  HTTPS_PROXY are supplied through `env -i`. No live GitHub traffic or writes.
- This exercises real parsing, transport, normalization, enrichment, SQLite,
  application refresh scheduling and terminal rendering. No application/model
  messages, fake clock, source injection seam, or shortened poll floor.
- The bounded scripts are retained as evidence, not installed tooling. Reproduce:
  copy `harness.py` to `/tmp/orgtop-round2.py`, `extra.py` to
  `/tmp/orgtop-round2-extra.py`; run `python /tmp/orgtop-round2.py`, then
  `python /tmp/orgtop-round2-extra.py`. Run in a disposable Linux environment
  with tmux/OpenSSL/Python and the built binary. Each runs finite local fixture
  servers and private tmux sockets, then shuts them down. Their temporary roots
  are reported in `/tmp/orgtop-round2{,-extra}-evidence/runtime.json`; remove them
  after inspecting evidence. The expected T-122 assertions remain red.
- `process-run.log`, `results.json`, `requests.json`, and the `extra-` equivalents
  are the final executed runs. Request logs retain only API/Accept headers and a
  credential-match boolean, never credential values. All requests are bounded GETs.
  All 66 main and 8 supplemental requests pass header checks. The main run has
  69/70 passing assertions and the supplemental run 12/13: both red assertions
  reproduce the same T-122 defect, not two separate findings. Harness exit 0
  means execution completed, not that every assertion passed.
- `entity-*.txt` and representative other `.txt` files are real tmux captures.
  The two review PNGs retained in the testing thread rasterize actual capture
  text with ImageMagick/DejaVu Sans Mono, then were inspected. They are not proof
  of real terminal font/color capability; code-point evidence proves the leak.

## Executed matrix

| Actual process flow | Result |
| --- | --- |
| T-119: four help/version aliases × unknown-before, unknown-after/reset, missing repo, malformed repo | 16 informational exits 0; expected output streams |
| T-120: reset combined with four boolean assignments × true/false | Eight usage exits 2, cache sentinel unchanged |
| T-120: `--reset-cache --no-cache=false` against populated real SQLite | Usage exit 2; SHA-256 identical before/after |
| T-121 valid branch with U+202E, Stream/detail 160x32 and Stream 40x12 | Visible uppercase escapes wide, no raw bidi at either width |
| Cold enrichment and warm second process | Both memberships complete; warm adds no commit HTTP request |
| Two simultaneous clients sharing cache | Both current; active reset refuses in under 3 seconds, DB retained |
| Real SQLite `BEGIN IMMEDIATE` writer contention | Current complete membership plus CACHE DEGRADED; responsive quit |
| Valid idle reset then fresh process | DB removed, rebuilt, complete membership reacquired |
| Real POSIX admission byte-region lock contention | Current complete membership plus CACHE DEGRADED; after release and 61 seconds, next real refresh clears degradation |
| Organization oversized 101-record response | Whole selection ERROR, no exact/partial repository polls |
| Cross-host next link | Whole selection ERROR; no link following or repository polling |
| First selector valid, second selector 503 | Atomic ERROR; no successfully fetched subset or exact polls |
| Conflicting canonical duplicate listing records | ERROR, no polls |
| Disabled-only organizations plus one exact repo | Exact repo retained and polled; no expanded repo |
| Two organizations, five full 100-record pages, one exact repo | Listing order alpha1,beta1,alpha2,beta2,alpha3; 20 polls: exact plus alpha r000–009 and beta r000–008 |
| Capacity/pagination disclosure | 481 known eligible omitted, plus more eligible may be omitted; 19 expanded/1 exact |
| Six repositories × 100 source events | Global newest-500-of-600 disclosure; no path enrichment needed |
| 50 scopes, sparse membership, Overview overflow | 1–38 of 50 rows; restored accounting after repeated Rain resize |
| 100 events × 50 confirmed overlapping scopes | 5,000 Rain candidates, 500 admitted, 4,500 omitted; page counts consistent with 10 per scope: 320 hidden items with 18 visible scopes, 360 with last 14 |
| Rain page traversal after resize | 1–18 → 19–36 → 37–50 → back to 1–18; stable tail admission and paused state |
| Rain sizes | 240x40 → 80x24 → 40x12 → 20x6 → 8x3 → 1x1 → 240x40; restored and responsive |
| Interesting Now under 100 overlapping events | 20 stored; 40x12 shows 4, hides 16, omits 80; source overlap does not duplicate entries |
| Full source success → one-repo failure after the other succeeds | Prior snapshot remains STALE, no partial new branch/event publication |
| Source recovery while enrichment becomes 403 | New current snapshot, repository rows preserved; path has 1 unknown and PATH ?1 |
| Subsequent enrichment recovery | Complete memberships return; PATH ?1 clears independently; no stale badge |
| Hostile malformed entity reference, multiple fresh processes | **T-122: raw U+202E in Interesting Now; detail correctly escapes** |
| Every PTY scenario | q exits successfully; finite loopback server stopped |

The source cycle waits 61 seconds at each of three real retry boundaries; the
admission-lock cycle waits 61 seconds. Manual `r` does not accelerate the floor.
Current-state assertions use an updated snapshot and absence of LOADING/ERROR/
STALE, because the shipped chrome omits the literal CURRENT marker.

## Confirmed finding and exclusions

T-122 is anchored to RG-012. `hostile-entity.json` deliberately contains a
non-hexadecimal `payload.head`, with U+202E. This accepted malformed presentation
payload reaches `interestingEntry.entityText()` unchanged, and `form()` appends it
without escaping before measuring. `entity-check.json` compares actual Rain and
detail captures: Rain contains raw U+202E, detail only `\u{202E}`.

This is not a claim that GitHub produces invalid commit IDs. RG-012 explicitly
requires presentation data never become control/bidi output, including malformed
source strings. T-121 fixed a valid branch description and the shared actor
consumer. Its rejected hypothesis that a branch became the entity reference is
still rejected; this test changes the actual head field. No open T-071/T-106/T-118
duplicates this defect. No broader security exploit is claimed.

Initial diagnostic harness assertions incorrectly required a literal CURRENT,
the phrase `50 scopes` instead of `scopes 1-38 of 50`, and an aggregate `2 activity`
rather than three per-scope `1 activity` rows. Captures contradicted those
assertions; they were corrected and the complete process matrix rerun. Space and
Right did not exercise Rain pause/paging; the final supplemental run uses `p` and
`[`/`]`. No product bugs were filed for these harness mistakes.

## Checks and limits

- Focused existing CLI/hostile-summary regression tests passed with
  `mise exec -- go test ./cmd/orgtop ./internal/cli -run 'TestInformational|TestBoolean|TestExecutableBoolean|TestHostileSummaryFlow' -count=1`.
- Full `mise exec -- task check` exited 0, including Go tests, lint, vet,
  distribution fixtures, coverage validation, and release configuration check.
  `mise exec -- taskrail validate` reports `state valid`; `git diff --check` passes.
  Commit/push hook results are recorded in the delivery summary. Distribution
  commands inside the full check use fixtures, not live releases.
- Only one new todo task was created through `taskrail task new`; no start,
  verify, or completion transition was performed. The generated STATE change is
  only its timestamp and todo count. Installed-skill version warning remains
  nonfatal; no unrelated skill refresh was performed.
- Not exercised: 15-minute due organization re-expansion, all combined rate-limit
  states, 100-scope maximum, every cache crash/tombstone transition, host ACLs,
  exhaustive Unicode/terminal matrix, live GitHub account/access semantics,
  live release artifacts, extension/Homebrew installation or maintainer sign-off.
  Linux amd64 only: no macOS, Windows, arm64, or live distribution-channel claim.
- These checks do not complete T-106, prove absence of other defects, or authorize
  a release. Only task/evidence changes are committed; product behavior remains
  unchanged and T-122 remains reproducibly failing for the next worker.
