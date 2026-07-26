---
name: review-explain-change
description: Prepare a technical change for human review by explaining behavior, limits, evidence and rollback implications. Use for pull requests, model or pipeline revisions and technical recommendations.
---

# Review and Explain Change

Make a proposed change understandable to the reviewer who must judge its
consequences. This skill produces a review packet; it does not merge, deploy,
approve data use or claim that a native runtime action happened.

## Interaction profile

Resolve the canonical `interaction-profile` before preparing the explanation.
Standard users receive the intended outcome and one safe reading path;
advanced and power users can receive interfaces, diffs and diagnostic evidence.
The profile never changes review requirements or data boundaries.

## Workflow

1. Explain the user or operational problem and the observable behavior change.
2. Name files, contracts or artifacts that changed through pointers rather than
   copying their full bodies into the review summary.
3. Separate implementation facts from hypotheses and unvalidated claims.
4. Report tests, checks and manual evidence with their scope and result.
5. State compatibility, migration, rollback and data-boundary implications.
6. Identify the reviewer decision required and the safest next action if the
   change is not approved.

## Invariants

- Never use a bundle selection as approval to merge or deploy.
- Never hide failed, skipped or unavailable checks.
- Never include credentials, source records or client-identifying content in a
  managed review artifact.
