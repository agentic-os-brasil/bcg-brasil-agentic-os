---
name: handoff
description: Prepare a compact current-request handoff for an unfinished piece of work and distinguish it from durable execution recovery. Use for “hand this off”, “save my place”, “prepare a resume note” or “what should the next session know?”.
---

# Handoff

Resolve the canonical `interaction-profile` before responding. It changes
presentation only; it never reads hidden session context or writes execution state.

## Contract

- Accept only work state, decisions, artifacts and blockers supplied in the
  current request.
- Return goal, completed work, pending work, next step, artifact references and
  open questions in a compact handoff.
- Label the result as a draft unless an existing execution ledger item is
  explicitly resolved through its governed capability.
- Do not create a checkpoint, pause/resume work, read a prior session or claim
  durable recovery.

## Completion

For long-running work, the execution ledger remains canonical: opaque pointer,
fenced attempt, revision check, metadata-only evidence and done contract. This
skill only prepares information a user may choose to submit to that flow.
