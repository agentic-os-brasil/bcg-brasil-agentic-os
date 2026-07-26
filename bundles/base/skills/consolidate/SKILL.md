---
name: consolidate
description: Review a user-supplied list of records for duplicates, staleness and contradictions, then propose a ranked cleanup plan without editing anything. Use for “consolidate this”, “what is stale?”, “clean up these notes” or “find drift”.
---

# Consolidate

Resolve the canonical `interaction-profile` before responding. It changes
explanation depth only; it never grants broad atlas, workspace or memory access.

## Contract

- Accept only records, summaries or indexes supplied in the current request.
- Return at most five ranked cleanup proposals with the evidence, owner and
  expected benefit for each.
- Separate duplicate, stale, conflicting and unlinked records.
- Do not scan storage, archive items, merge records, change memory or claim a
  cleanup was applied.

## Completion

Return proposed changes for review. Canonical memory hygiene and any approved
write remain separate, authorized capabilities.
