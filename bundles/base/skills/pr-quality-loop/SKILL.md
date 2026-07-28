---
name: pr-quality-loop
description: Orchestrate an evidence-gated pull-request quality loop that reuses fresh metadata, routes findings to the author and ends in a merge-readiness report.
---

# Pull-Request Quality Loop

Run the cheapest reliable sequence before expensive review. The loop joins
`qa-gate` and `pr-review`, but it never merges a PR or edits another
contributor's branch.

## Interaction profile

Resolve the canonical `interaction-profile` before presenting the loop report.
The profile changes explanation detail only; it does not change gates,
evidence or authority.

## Sequence

1. Create or reuse a `PR_EVIDENCE_PACK` for one head revision. Refresh only
   stale fields; do not recollect the same metadata at every step.
2. Apply Stage 0 from `pr-review`: draft, staleness/conflict,
   mergeability, current-head checks, human review and required evidence.
3. Read or await required automated signals only through an authorized
   adapter. A missing, timed-out or malformed signal is `unavailable` (or
   `HOLD` when the repository marks it required), never a green result.
4. Choose quick, standard or deep review from risk before reading the full
   diff. Do not spend deep-review effort on a deterministic blocker that must
   be fixed first.
5. Run `pr-review` and `qa-gate` at that depth. Classify findings as blocker,
   non-blocker, false positive or question, with a pointer and owner route.
6. Return blockers to the PR author. Do not modify their branch unless the
   user explicitly changes ownership and scope.
7. When the head changes, invalidate the pack and all head-bound evidence.
   Re-run only the affected gates, then focused tests and the smallest broader
   suite that protects the change.
8. End with `APPROVE`, `REFINE`, `HOLD` or `UNAVAILABLE`, plus the exact
   evidence still missing and one next action. Merge remains separate.

## Evidence packet minimum

Use pointers for repository, PR number, base/head revisions, changed files,
draft/mergeability, required checks, human reviews, automated signals, risk
triggers, focused hunks, commands and timestamps. Do not persist source bodies,
credentials, client data or full tool output.

## Anti-patterns

- starting with a full diff or expensive worker before Stage 0;
- rerunning fresh checks without a revision or reason;
- treating an automated reviewer as the decision maker;
- declaring merge-ready while required evidence is stale or unavailable;
- merging inside the loop.
