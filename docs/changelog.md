# Changelog Policy

How `CHANGELOG.md` entries are written in this repository. `AGENTS.md` and
`README.md` link here instead of restating the policy.

## When to add an entry

- Add an entry under `## [Unreleased]` for user-visible behavior changes only.
- Skip internal-only refactors, CI and toolchain plumbing, test-only work, and
  routine dependency bumps with no user-visible effect.
- Fold one user-facing change into one entry even when it spans several tracked
  tasks.
- Put the entry under the Keep a Changelog section that describes its effect:
  `Added`, `Changed`, `Deprecated`, `Removed`, `Fixed`, or `Security`.
- The changelog edit belongs in the same commit as the change it describes, not
  in a follow-up chore commit.

## How to write it

- Keep entries terse: one or two sentences.
- Lead with the command, flag, or observable behavior, and state what changed
  for the person running OrgTop.
- Leave out function names, package layout, internal schemas, test strategy, and
  design rationale. Those belong in the commit body, the task, or the spec.
- Describe security boundaries precisely without turning the entry into a
  threat-model essay.
- Copy-edit against the existing entries so tone and length stay consistent.

## Examples

- Good: `` `--scope` now rejects a repository selector that names no repository,
  instead of starting with an empty activity view. ``
- Bad: a paragraph naming the internal functions involved, every field compared,
  the test layers added, and the implementation history behind the change.

## Preparing a release

Release notes are extracted from the `## [<version>]` section matching the tag,
and `.github/workflows/release.yml` refuses to publish a tag whose section is
missing or empty. Before tagging:

1. Move the `## [Unreleased]` entries under a new `## [<version>] - YYYY-MM-DD`
   heading.
2. Leave an empty `## [Unreleased]` section above it.
3. Update the comparison links at the end of `CHANGELOG.md`.

Rehearse the guards locally with `scripts/check-changelog-version.sh <tag>`;
`task test:changelog` tests the guards themselves.
