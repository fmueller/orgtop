---
id: T-021-stop-restating-the-entity-class-in-event
title: Stop restating the entity class in event descriptions
status: completed
priority: medium
spec_ref: specs/v0.1.0.md#github-activity-source
dependencies: []
updated_at: "2026-08-24T07:18:10Z"
---

# T-021-stop-restating-the-entity-class-in-event Stop restating the entity class in event descriptions

## Description

A Stream row names the category in its own column and then names it again inside
the description, because `internal/github/normalize.go` builds descriptions that
repeat the entity noun:

```
acme/frontend  pull request  bob · opened pull request #42
acme/backend   review        carol · approved pull request #41
acme/infra     comment       dave · commented on pull request #17
```

`subject(verb, entity, ref)` renders `"<verb> <entity> <ref>"`, so every
pull-request row spends roughly thirteen columns restating what the category
column already said. At the `40x10` contract that is width the description cannot
spare, and the duplication reads as noise rather than detail.

The verb is the part that carries information and must stay: `opened`, `merged`,
`closed`, `approved`, and `requested changes on` are not derivable from the
category. Only the entity noun is redundant.

## Acceptance

- Descriptions carry the verb and the entity reference without the entity noun:
  `opened #42`, `merged #42`, `approved #41`, `commented on #17`.
- A description with no entity reference still names the entity, so it does not
  degrade to a bare verb: an event with no pull-request number keeps a readable
  phrase.
- Issue-only comments stay distinguishable from pull-request comments, since the
  category is `other` for the former and the reference alone would not say which.
- Push descriptions are unchanged: `push` and `pushed 3 commits to main` do not
  duplicate an entity noun, and the branch and commit count are the detail.
- `other` descriptions are unchanged.
- The merge-specific description FR-005 requires survives the rewording.
- Category remains named exactly once per rendered row.
- No public API or domain field changes; only the description text.

## Test Expectations

- Extend the existing normalization tests in `internal/github` rather than adding
  a parallel harness; they already assert description text per payload type.
- Cover every verb branch of `pullRequestVerb` and `reviewVerb`, plus the
  no-reference fallback and the issue-versus-pull-request comment split.
- A `internal/tui` or `cmd/orgtop` assertion that a rendered row names its
  category once.

## Verification Notes

- Record `task check` and `go test ./... -race` exit statuses.
- Show a before and after render of the same synthetic snapshot, with the column
  width recovered on pull-request rows.

## Implementation Notes

- Seam is `subject`, `pullRequestDescription`, `reviewDescription`, and
  `commentDescription` in `internal/github/normalize.go`.
- FR-005 was amended for this task: a description names what the event did and to
  which entity, not the class the category already names.
- Descriptions are domain data, not rendering. Do not make the wording depend on
  terminal width; the layout ladder in `internal/tui/stream.go` handles width.
- 2026-08-24T07:18:04Z: verification pass
