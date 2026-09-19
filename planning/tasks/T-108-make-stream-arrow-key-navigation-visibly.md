---
id: T-108-make-stream-arrow-key-navigation-visibly
title: Make Stream arrow-key navigation visibly responsive before v0.2.0
status: todo
priority: high
spec_ref: specs/v0.2.0.md#rg-012-mixed-scope-presentation-contract
dependencies: []
updated_at: "2026-09-19T08:22:42Z"
---

# T-108-make-stream-arrow-key-navigation-visibly Make Stream arrow-key navigation visibly responsive before v0.2.0

## Description

The maintainer reports that Page Up/Down works but arrow keys appear to work
only intermittently. A deterministic reproduction confirms invisible keyboard
focus in Stream. Treat this as a v0.2.0 release blocker and verify it during
T-106 before tagging, rather than deferring it to the following release.

`Model.handleKey` in `internal/tui/model.go` dispatches arrows and page keys
through the same path. In `internal/tui/stream.go`, `stream.scrolled` moves
focus; `stream.contained` moves the viewport only when focus crosses its
boundary. However, `stream.render` renders plain rows and the viewport without
indicating the focused index. Valid arrow presses inside the visible window
therefore change internal state without changing screen content. Reversing
direction after scrolling has the same problem. Page keys cross the viewport
boundary sooner, making them appear more reliable.

RG-012 and A-078 explicitly require this focus-based navigation, so the
boundary behavior itself is intentional; missing feedback is the defect.
README Controls also misleadingly describes arrows as always scrolling one
row and page keys as always scrolling one page. Overview and open Stream
detail use direct offset scrolling and do not have this specific defect.

## Acceptance

- Every non-clamped Stream focus change is visibly identifiable, including
  movement inside a stationary viewport and reversal after scrolling. The
  focused-row indicator works without color and with ASCII fallback.
- Preserve RG-012/A-078: arrows move focus by one, page keys by available
  event-row capacity, and the viewport moves only enough to reveal focus.
  Enter opens the visibly focused event; Escape restores focus and viewport.
- Preserve bounds, event content, and Scope context at narrow widths,
  including 40x10. Refresh, resize, view switching, and top/bottom clamps
  never leave a missing or incorrect focus indicator.
- Keep Overview and detail direct scrolling unchanged. Correct README controls
  to distinguish focus navigation from scrolling and document Enter/Escape.
  Record the user-visible fix in CHANGELOG.md.
- Verify this finding during T-106 release-candidate testing before tagging
  v0.2.0. This is required pre-release work, not a post-release enhancement.

## Test Expectations

- Add a regression test comparing rendered focus feedback before and after
  one arrow press while offset remains unchanged. Assertions about internal
  focus or offset alone cannot detect this defect.
- Cover upward reversal, both list boundaries, Page Up/Down, and Enter opening
  the indicated event; include no-color/ASCII and narrow-terminal output.
- Check focus retention across resize, refresh, and view switches.
- Run targeted TUI tests and `task check`. Inspect representative terminal
  renders and exercise arrows/page keys in a real terminal during T-106.

## Verification Notes

- Investigation on 2026-09-19 used 20 deterministic events and terminal height
  10, leaving six Stream event rows, starting at focus 0 and offset 0.
- A temporary diagnostic using existing `streamModel`, `numberedEvents`,
  `apply`, and `scrolled` helpers sent keys through `Model.Update` and compared
  full `View().Content` strings. The first five Down presses reached focus
  1 through 5 with offset 0 and byte-for-byte unchanged rendered output.
  The sixth reached focus 6, offset 1, and changed the render. Up then reached
  focus 5, offset 1, with unchanged output again. Page Down then reached focus
  11, offset 6, and changed the render. The diagnostic was removed afterward.
- `mise exec -- go test ./internal/tui -run
  'TestArrowDiagnostic|TestStreamScrollKeysWindowTheEventsWithinBounds|TestOverviewScrollKeysWindowTheRowsWithinBounds|TestStreamDetailReachesEveryWrappedLine'
  -v` passed. Existing Stream tests intentionally expect a stationary viewport
  within the window but do not require visible focus feedback.
- No live terminal-byte capture was performed. This confirms a Stream feedback
  defect, not that every reported symptom has the same cause. If Overview or
  detail also intermittently ignores arrows, investigate terminal input
  separately during the real-terminal reproduction.

## Implementation Notes

- Prefer making existing focus visible over unconditional viewport scrolling.
  Changing navigation semantics instead requires explicit alignment of RG-012
  and A-078 first.
- This task records analysis only; no production fix has been applied.
