# OrgTop Specifications

This directory contains versioned normative product contracts for OrgTop.

## Reading Order

1. Read `v0.1.0.md`, the shipped contract for the first useful local release.
2. Read `v0.2.0.md`, the active implementation-ready contract for monorepo Scopes and Rain.
3. Read `v0.3.0.md`, the Draft contract for explainable intelligence.
4. Read `v0.5.0.md`, the Draft contract for shared real-time architecture.
5. Read `v1.0.0.md`, the non-normative validation and product-vision contract.

Later versions inherit shipped invariants only when their own contracts say so.
Roadmap ideas are not implementation requirements until represented by an active
versioned spec and eligible Taskrail work.

## Authoring Conventions

- `Draft` specs are exploratory, `Implementation-Ready` is a closed contract awaiting
  implementation, `In Progress` is an active implementation baseline, and `Done` is
  contract-locked.
- Keep functional requirements, non-functional requirements, acceptance scenarios,
  and explicit non-goals distinguishable.
- `###` headings under `## Potential Features` are Taskrail coverable areas.
- Tasks must reference live headings reported by `taskrail spec show --anchors`.
- Change active versions and managed execution state through Taskrail, not manual
  state edits.
