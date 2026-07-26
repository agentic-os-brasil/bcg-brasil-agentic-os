---
name: decision-log
description: Draft a durable decision record from user-supplied context, options and rationale without writing it anywhere. Use for “record this decision”, “capture the rationale”, “what should go in the decision log?” or “make this decision reviewable”.
---

# Decision Log

Resolve the canonical `interaction-profile` before responding. It changes
presentation only; it never authorizes a workspace or repository write.

## Contract

- Accept only the decision, context, alternatives and source references supplied
  in the current request.
- Return a proposed decision record with scope, rationale, consequences,
  assumptions, review trigger and unresolved questions.
- Distinguish a decision from a task, status update or hypothesis.
- Do not assign a canonical code, append a file, alter prior history or claim
  that the decision was recorded.

## Completion

Return a reviewable draft. A future canonical writer must use the owning
decision store, confirmation, append-only history and its own receipt contract.
