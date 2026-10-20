---
id: T-106-test-the-v0-2-0-release-candidate-before-tagging
title: Test the v0.2.0 release candidate before tagging
status: completed
priority: high
spec_ref: specs/v0.2.0.md#integration-documentation-and-release-readiness
dependencies:
    - T-070-document-the-closed-v0-2-0-behavior
    - T-068-verify-distribution-channel-parity
updated_at: "2026-10-20T19:12:31Z"
---

# T-106-test-the-v0-2-0-release-candidate-before-tagging Test the v0.2.0 release candidate before tagging

## Description

The maintainer's own testing pass over the closed v0.2.0 behavior, run against
a real build on real hosts before any v0.2.0 tag exists. It is deliberately not
automated: it covers what the suites cannot reach, which is a person using
OrgTop against live GitHub data in a real terminal.

A completed version is permanently non-reusable
(`scripts/distribution-ledger-append.sh`), so a defect found after the tag
publishes costs the version number rather than a retry. This task exists to
find those defects while the cost is still a commit.

## Acceptance

The original checklist below is retained for traceability. For this candidate,
the maintainer accepted the supplementary video review and requested closure
with the remaining checks waived as described under Maintainer Acceptance.
Completion records that decision, not execution or passing of the waived checks.

- The integrated binary is exercised against live GitHub data with a real
  token, in both Overview and Stream, and in the Rain and Interesting Now
  views, at the credential precedence the documentation states.
- The unified `--repo`/`--path` and organization selectors are exercised with a
  repository selector, a path selector, an organization selector, and a mixed
  set, including selectors that name nothing and selectors the token cannot
  see.
- Enrichment and the SQLite cache are exercised across a cold start, a warm
  start, a cache the process cannot write, and a cache left behind by an
  earlier version.
- Degraded and responsive behavior is checked on a narrow terminal, a short
  terminal, a terminal without colour, and a terminal whose charset cannot
  render the glyphs.
- Every installation channel is installed from and launched on at least one
  host: the release archive, `gh extension install`, and `brew install`.
- Findings are recorded as tracked tasks rather than fixed silently, and each
  one is either closed or explicitly accepted before v0.2.0 is tagged.

## Test Expectations

- Manual, on real hosts, against live GitHub. The automated gate is `task
  check`; this task is what `task check` cannot see.
- Prefer the platforms the release matrix publishes; record which of the six
  were actually covered and which were not.

## Verification Notes

- Record the hosts, terminals, token precedence paths, and selectors
  exercised, the channels installed from, and every finding with its task id.
- Record explicitly which matrix targets went untested, so the release decision
  is made against a known gap rather than an assumed one.

## Maintainer Acceptance

On 2026-10-20 the maintainer said, "I watched the video, looks very good," then
"I agree with your recommendation. Does that mean you can actually declare this
task as finished and move on with the loop?" The source conversation interpreted
that agreement as acceptance of the video review plus waiver of the outstanding
checks, and directed this task's evidence-only closure on that basis. The
maintainer did not enumerate each waived check individually.

- Acceptance and direction: [source conversation](https://ampcode.com/threads/T-01a15a92-3b8e-756c-820c-8b39438af199).
- Supplementary evidence: [recording conversation](https://ampcode.com/threads/T-01a15a9b-3980-71da-9070-e4df5f723fb2).
- No tagging, publication, release workflow trigger, or future automation is
  part of this closure. The v0.2.0 release-readiness task remains separate.

### Executed Supplementary Evidence

Tested source: [candidate commit](https://github.com/fmueller/orgtop/commit/630ff048b8daaec990ba4c9e07a62ecfed290d9e).
Unmodified dev binary (`orgtop dev`), Linux amd64 orb, Go 1.26.7,
xterm 379/tmux 3.3a; actual terminal frames captured with ffmpeg 5.1.9.
Transport used synthetic GitHub fixtures and credentials, not live GitHub.

- Build: `mise exec -- env CGO_ENABLED=0 go build -trimpath -o /tmp/orgtop-demo ./cmd/orgtop`.
- Binary SHA-256: `c320548c1fb25789526b9b53474c3600f838cf66dee61f42dbf40e2b000b5e5d`.
- Recording-thread gate: `mise exec -- task check`, exit 0.
- In the recording checkout's `.amp/in/artifacts/`, all three final videos
  were inspected: `orgtop-01-navigation-rain.mp4` (29.4s),
  `orgtop-02-resize-ascii.mp4` (26.067s), and
  `orgtop-03-cache-degradation.mp4` (19.134s). They show navigation/detail,
  Rain/Interesting Now, pause/window changes, resize/collapse/restoration,
  ASCII category glyphs and degraded-cache operation without observed glitches.
- `orgtop-terminal-evidence.json` (22,768 bytes) retains assertions, requests,
  captures, provenance and limitations. Retrieved and inspected for this closure;
  SHA-256: `22f255c42e90e5bbca202ddf00cab6a16976b194e63cd56d889057d61f68802d`.
  Media and the full export remain in the recording thread, not in this commit.
- Raw harness: 24 PASS, one FAIL. The failed assertion, "NO_COLOR has no styled
  terminal cells," was invalid: a follow-up found bold/reset and default
  foreground/background SGR only, no selected colours. Bold is allowed. This
  is not a 25/25 passing run or a confirmed product defect. Decorative middle
  dots remain Unicode; fully ASCII-only output was not established.
- Cold/warm reuse (one evidence fetch, none additional warm), no-cache (no DB),
  non-directory cache degradation and synthetic schema-99 degradation passed;
  schema 99 was preserved. Repository/path counts remained 3 / 2 confirmed
  plus 1 unknown. No product fix or new defect task resulted from this pass.

### Maintainer-Accepted Gaps — Not Executed

- Live GitHub with real credentials and documented credential precedence.
- Full repository/path/organization/mixed/empty/inaccessible selector matrix.
- Personal real-host terminal/capability checks and real-host cache permissions;
  non-directory failure is not evidence of an unwritable-directory ACL test.
- Cache genuinely produced by an earlier version; schema 99 is synthetic,
  not historical-cache evidence.
- Actual release-archive, GitHub CLI extension and Homebrew installation/launch.
  The v0.0.2 rehearsal was withdrawn; no replacement release was made here.
- Platform coverage: only Linux amd64 orb executed. Linux arm64, macOS
  amd64/arm64 and Windows amd64/arm64 were untested in this supplementary pass;
  real-host coverage, including Linux amd64, is waived rather than claimed.

### Closure Workflow Evidence

- Documentation/evidence only: no product behavior changed; red/green TDD and
  new behavior tests are not applicable. No future automation was implemented.
- Initial `mise exec -- task check`, `mise exec -- taskrail validate` and
  `git diff --check` passed in this closure checkout.
- Dedicated Task loaded `code-simplifier`: no edits recommended; validation
  and diff checks passed.
- Independent General Task loaded `code-reviewer` and its ECC General guidance:
  "No concrete task-relevant findings." No specialist lane applies to prose
  and Taskrail-generated state; no product code, auth or persistence changed.
- Fresh candidate-validation Task loaded `code-reviewer`: validated findings
  empty, no rejected or duplicated candidates. No fixes or deferrals required.
- Final checks and fresh disposition verification precede Taskrail verification
  and completion; their results are recorded in the lifecycle verification.

## Implementation Notes

- Do not tag v0.2.0 in this task. Tagging is the decision this task informs.
- The distribution channels can only be installed from once a version is
  published, so the channel bullet is satisfied against the v0.0.2 rehearsal
  release or against v0.2.0 itself after tagging, whichever the maintainer
  chooses; record which.
- 2026-10-20T18:32:21Z: Requires maintainer personally testing integrated candidate on real hosts/terminals with live GitHub and real token; existing docs/testing/round1-report through round5/report.md evidence uses synthetic orb fixtures, not acceptance. Return host/terminal, credential precedence, repo/path/org/mixed and empty/inaccessible selectors, cold/warm/unwritable/earlier-version cache, narrow/short/no-color/charset, three-channel install/launch, findings task IDs and six-target coverage evidence. v0.0.2 rehearsal was withdrawn (docs/withdrawals/v0.0.2.md); v0.0.2 and v0.2.0 release reads report not found. Historical rehearsal install evidence or separately authorized post-tag channel evidence is needed; no tagging/publication authorized. Stop outer loop; do not advance T-118/T-071.
- 2026-10-20T19:02:40Z: Maintainer accepted supplementary video review and requested closure; remaining live-GitHub, real-host/platform, credential/selector, genuine historical-cache and installation-channel checks are accepted gaps, not executed tests. Resume T-106 only for evidence/documentation closure.
- 2026-10-20T19:12:22Z: verification pass
- 2026-10-20T19:12:31Z: Maintainer accepted video review and waived outstanding original checks; completed documentation/evidence closure only, not real-host/live-GitHub/channel test execution. Required simplification, General review, candidate validation, final checks and fresh disposition verification passed; no product change or release authorization.
