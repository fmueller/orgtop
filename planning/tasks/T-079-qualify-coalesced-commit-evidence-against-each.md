---
id: T-079-qualify-coalesced-commit-evidence-against-each
title: Qualify coalesced commit evidence against each event's before object
status: completed
priority: high
spec_ref: specs/v0.2.0.md#changed-file-enrichment
dependencies:
    - T-052-implement-bounded-enrichment-coordination
updated_at: "2026-09-03T14:33:53Z"
---

# T-079-qualify-coalesced-commit-evidence-against-each Qualify coalesced commit evidence against each event's before object

## Description

RG-005 requires that a coalesced commit result satisfies an event only when that
event's own before object equals the sole parent the adapter verified. Nothing
performs that check: `domain.EvidenceOutcome.ForSoleParent`
(`internal/domain/outcome.go:149`) exists for exactly this rule and has no
production caller. The before object is dropped at classification, so the fact
the check needs is gone by the time evidence settles:
`domain.NewCommitEvidence` (`internal/domain/evidence.go:149`) stores only the
repository and head, `domain.Event` (`internal/domain/event.go:35`) carries no
before object, and `github.pushEvidence` (`internal/github/evidence.go:47`)
discards the before SHA once it selects the one-commit form.

T-052 shipped the coordinator that coalesces `commit(repository-key,head)` into
one work key and preserves the verified sole parent on every delivered outcome
(`internal/enrichment/coordinator.go` `forDescriptor`), so the proof is
available; the per-event qualification is what is missing.

## Acceptance

- Each event requesting a coalesced commit identity receives the complete path
  set only when its own before object equals the verified sole parent; every
  other event sharing that head is incomplete rather than guessed.
- The before object reaches the qualification point without widening the
  evidence work key: two events sharing one head still spend one request.
- Cache-served commit evidence is qualified by the same rule as freshly
  acquired evidence.

## Test Expectations

- Two events sharing one commit head with different before objects: one is
  complete and the other incomplete, from a single request.
- A cache hit for a commit identity is qualified the same way.

## Verification Notes

- Record the request count proving the identity still coalesces.

## Implementation Notes

- Discovered by the T-052 review pass; see `internal/enrichment/coordinator.go`
  and `internal/domain/outcome.go:149`.
- Changing `domain.NewCommitEvidence` or `domain.Event` is a domain surface
  change and was deliberately kept out of T-052's narrow scope.
- 2026-09-03T14:33:36Z: verification pass
- 2026-09-03T14:33:53Z: Verified by task check, focused race tests, fresh/cache coalescing tests, simplifier pass, and independent review with no findings.
