---
id: T-023-disclose-what-the-stream-list-covers-and-mark
title: Disclose what the Stream list covers and mark shortened content
status: todo
priority: medium
spec_ref: specs/v0.1.0.md#stream-view
dependencies:
    - T-022-give-stream-sticky-column-headings
updated_at: "2026-08-23T20:14:11Z"
---

# T-023-disclose-what-the-stream-list-covers-and-mark Disclose what the Stream list covers and mark shortened content

## Description

Two ways Stream leaves a user unsure what they are looking at.

First, nothing says how much activity the list represents. A refresh fetches one
`per_page=100` page per repository and the snapshot keeps the newest 500 events
(`domain.MaxSnapshotEvents`). A user cannot tell whether a short list means a
quiet week or a bounded fetch, nor whether scrolling to the bottom means they have
seen everything or merely everything retained.

Second, content wider than the terminal is cut without a mark. `truncate` in
`internal/tui/chrome.go` drops characters silently, so at 56 columns a row reads:

```
14:38  acme/backend   push  alice · pushed 3 commits to 
```

`pushed 3 commits to ` looks like the whole description with a trailing space, not
a shortened one. The reader has no signal that a branch name was removed.

## Acceptance

- Stream states the number of events the current snapshot holds.
- When the FR-006 bound discarded events, Stream says the list is bounded rather
  than simply ending; when it did not, it makes no such claim.
- The snapshot carries whether the bound truncated it, so the view reports a fact
  rather than inferring one from the count reaching the limit.
- The disclosure gives way before the event rows and the quit hint at tight sizes,
  and A-010 continues to hold.
- Content shortened to fit the width is marked as shortened, and the mark is
  counted against the available width so a marked line is never wider than an
  unmarked one.
- Content that fits is never marked.
- The mark measures by rendered width, so a wide rune is not half-cut and the
  existing wide-rune assertions continue to hold.
- The explicit loading, error, and no-recent-activity states are unaffected.
- Overview shares the truncation behavior, since it shares the helper; its
  identity and count columns must stay readable.

## Test Expectations

- A `internal/domain` test that the truncation flag is set only when the bound
  discarded events, including exactly at the boundary.
- `internal/tui` tests for the marked and unmarked cases, the width accounting,
  and wide runes.
- A test that a bounded snapshot renders the disclosure and an unbounded one does
  not.
- Extend the existing narrow-size tests rather than adding a parallel harness.

## Verification Notes

- Record `task check`, `go test ./... -race`, and `git diff --check` exit statuses.
- Capture a render at a width that truncates, showing the mark and the unchanged
  total width.

## Implementation Notes

- Depends on T-022 because both change Stream's chrome; landing them in order
  avoids conflicting rewrites of the same layout code.
- FR-006 was amended so the snapshot retains whether the bound discarded events.
  `domain.NewSnapshot` already computes the cut in `internal/domain/snapshot.go`.
- `truncate` in `internal/tui/chrome.go` is shared by the header, the footer, and
  both bodies. Marking every caller is not obviously right: a truncated header
  field is already understood as tightened. Decide per caller and say why.
