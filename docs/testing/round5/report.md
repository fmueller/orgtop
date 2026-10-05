# Fifth and final adversarial executable test round

2026-10-05 UTC / Europe-Berlin. Pinned contract: `specs/v0.2.0.md`.
Fetched clean main and origin/main both equaled
[`ba56c5d`](https://github.com/fmueller/orgtop/commit/ba56c5dc5776664a201c7cb4eda5b206690231fb).
**912 executed assertions: 903 pass, nine fail, two new confirmed bugs.**
T-129 and T-130 are focused high-priority todo tasks, not implemented. T-119–T-128
stay completed. Maintainer T-106 and all other existing tasks are untouched.
This is round five of five; no sixth round is planned or authorized here.

## Method and reproducible commands

Linux amd64 orb; mise-pinned pure-Go CLI, real bounded loopback HTTPS CONNECT,
tmux PTYs, actual SQLite databases and locks, and wall-clock refresh scheduling.
Read prior round and fix evidence; reused committed harnesses and inspected their
producing commits. No model/application/clock injection, real credential, live
API write, release, deployment, or native Windows runtime simulation.
Disposable HOME/cache and synthetic token; CONNECT accepts only api.github.com:443.
The 448 logged GETs include matching API version/media/synthetic-auth checks;
logs retain only a credential-match boolean, never a credential value.

All commands ran from the repository root, under `mise exec --`:

```sh
env CGO_ENABLED=0 go build -o /tmp/orgtop-round2 ./cmd/orgtop
cp /tmp/orgtop-round2 /tmp/orgtop-t125
python docs/testing/round2/harness.py
python docs/testing/round3/harness.py
python docs/testing/round3/extra.py
python docs/testing/t122-entity-controls.py /tmp/orgtop-round2 final
python docs/testing/t123-files-array.py
python docs/testing/t124-pagination.py
python docs/testing/t125-retry.py
python docs/testing/t126-pagination.py
python docs/testing/t127/harness.py
python docs/testing/round4/harness.py
python docs/testing/round4/rate-headers.py
python docs/testing/round5/link-probes.py
python docs/testing/round5/concurrent-retry.py
task check
taskrail validate
```

The last two Python scripts executed from identical `/tmp/orgtop-final-probes.py`
and `/tmp/orgtop-final-retry.py` copies before being retained here. Independent
suites used separate fixture servers/tmux sockets. The failed T-125 command
stopped its shell's `&&` chain, so T-126/T-127 were explicitly run afterward;
both completed successfully. No red timing assertion was weakened or rerun away.
Results JSON was inspected independently of script exit status: older scripts
can return zero despite failed assertions. Three scripts returned 1 for the nine
recorded failures; other suites and the full local gate returned 0.

## Exact suite matrix

| Executed suite | Pass / total | Evidence prefix |
| --- | ---: | --- |
| Round2 CLI, bidi, organization, capacity, stale/recovery | 70 / 70 | regression |
| Round3 evidence/cache integrity and contention | 218 / 218 | cache |
| Round3 corrected-warm, recency, Rain/window, rate floor | 29 / 29 | extra |
| T-122 entity-control rendering | 7 / 7 | controls |
| T-123 required files and cold/corrected-warm acquisition | 64 / 64 | files |
| T-124 pagination and cache admission/reuse | 310 / 310 | pagination |
| T-125 120-second retry and recovery | 9 / 10 | retry |
| T-126 strict organization query parsing | 28 / 28 | organization |
| T-127 quoted Link, terminal admission, warm reuse | 15 / 15 | quoted |
| Round4 Link/query/cache/source-delay composition | 58 / 58 | composition |
| Round4 Retry-After/exhausted-reset composition | 6 / 6 | headers |
| New Link trust/completeness matrix | 85 / 91 | link |
| New concurrent 65/95-second retry constraints | 4 / 6 | concurrent |

Each prefix has exact `*-results.json` assertions and actual `*-requests.json`.
Detailed exercised boundaries:

- CLI: four informational aliases × four invalid/admin argument positions;
  four bare booleans × true/false with reset; twelve hostile CLI vectors;
  expected exits/streams, unchanged cache sentinel/populated DB, and no network.
- Controls: bidi branch in Stream/detail and constrained width; hostile entity
  head in Rain at 240x32, 80x24, 40x12, 20x6, detail and responsive quit.
- Enrichment: missing/null/wrong-shaped commit/compare files; page-two missing,
  denied/rate; explicit empty arrays; 299 versus 300 comparison paths; invalid
  status/rename/duplicates; malformed/reverse/cyclic/duplicate-query Links;
  no-cache/cold/repeat/corrected/reused admission and request counts.
- Selection/capacity: oversized/cross-host/conflicting/partial organizations;
  exact precedence and five-page fairness; 600 events bounded to 500; 50 scopes;
  resize to 1x1 and restore; source success→503 stale retention→current unknown
  enrichment denial→complete recovery with real 61-second waits.
- SQLite: cold/warm/concurrent clients, active reset refusal/idle rebuild,
  BEGIN IMMEDIATE contention, future/foreign/zero-ID/schema corruption, invalid
  count/ordinal/time/expiry, tombstone/bootstrap and post-publication SIGKILL;
  query membership src/src2/SRC/glob and changed-parent applicability on one cache.
- Time/UI: future, fresh, 5m, 15m, 1h, 1d and week-old events; ten Stream rows;
  Interesting Now cutoff independent of paused Rain's 15m→available window.
- Retry: 61/119s no dispatch and eight-entity recovery; source 130s versus
  enrichment 120s floor; Retry-After 65 versus exhausted reset around 75s.
- New Link matrix: both commit and organization consumers × plain next,
  escaped-letter next, escaped-space relation list, escaped non-next last,
  cross-host, userinfo, fragment, encoded path, encoded duplicate page key,
  malformed query escape, cross-entity, and unclosed title. Valid headers also
  compose quoted commas/semicolons/escaped quotes in titles. Check request
  count/order, unknown versus confirmed membership, actual SQLite rows and
  corrected-warm acquisition. All invalid URL/header controls pass.

## Two reproduced defects

**T-129: escaped relation falsely proves completeness.** With `rel="ne\xt"`,
the binary makes one commit GET instead of two, caches `complete=1,path_count=1`
containing only `src/first.go`, and displays `No activity in retained snapshot`
for `other` instead of confirmed membership from `other/last.go` on page two.
Correcting upstream to plain next in a second process does not help: zero new
enrichment requests and the same cached false negative. Organization expansion
similarly polls only exact/keep and acme/r001, silently omitting acme/r002.
Six red assertions support one shared-parser task, not six tasks. Plain next,
escaped-space list and actual non-next controls pass. RFC 8288 section 3 and
Appendix B.4 define relation values after quoted-pair decoding. The defect is
not T-127's already-fixed quoted-title comma rejection.

**T-130: first concurrent rate response wins over a later deadline.** A barrier
puts two enrichment GETs in flight; responses instruct 65s and, 0.400007s later,
95s. No keyboard input occurs during the wait. Events resumes at +65.002226s
after the first response, 30.397781s before the later deadline, followed by both
commit requests and complete UI recovery. Two assertions fail. The existing
T-125 regression independently resumes at 119.998494s relative to its second
request, producing the ninth red assertion. The new large-gap fixture removes
ambiguity from that small timing discrepancy. Source inspection confirms that
`settleIdentity` records RetryAt only for the first rate-limited result.

Both tasks use real RG-003 anchors, expected/actual reproductions, acceptance and
regression criteria; T-129 also documents shared RG-010 and cache consequences.
No implementation changes were made and neither task was started or completed.

## Gates, delivery scope, and limits

`mise exec -- task check` passed the full local CI-equivalent gate (`check.txt`).
Taskrail validation and `git diff --check` passed after task creation/narratives.
Hooks installed and maintainer identity check passed. Only round5 evidence,
two supported Taskrail-created task files, and Taskrail's own state/count update
are delivery scope. Existing task contents and product code remain unchanged.

Native Windows CI 37251540577's six successful jobs were independently confirmed
before this assignment and were not rerun or claimed as local E2E coverage.
This round does not prove live GitHub/account behavior, packaged installation,
distribution-channel acceptance, every OS/terminal/font, exhaustive races or
power-loss safety, 15-minute organization re-expansion, or maintainer T-106
release sign-off. No mutation run was needed for evidence-only changes.
The Taskrail installed-skill/version warning remains nonblocking; no tooling
refresh was attempted. IO delegate preflight was unavailable, so bounded native
reads were used. Servers/tmux sockets and disposable test roots were cleaned up.
