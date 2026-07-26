---
name: meeting-to-work-items
description: Run a human review of proposed decisions, action items, follow-ups and risks from meeting notes before any future authorized write. Use for “close this meeting”, “review the actions from this transcript” or “prepare these notes for follow-up”.
---

# Meeting to Work Items

Resolve the canonical `interaction-profile` before responding. It changes
explanation depth only; it never authorizes task, decision, memory or connector
operations.

## Orchestration contract

- Accept only meeting notes or an `extract-work-items` proposal that the user
  supplies in the current request.
- Compose the `extract-work-items` method to create a proposal, then lead the
  user through correction and confirmation.
- Return a review result with items marked `confirmed`, `rejected` or
  `needs-clarification`.
- Do not call a task system, write a decision log, update a workspace, notify a
  person, read prior context or claim that any item was filed.

## Workflow

1. Run `extract-work-items` on the supplied content when a proposal is absent.
2. Present the smallest scannable set: decisions, actions, follow-ups and
   risks, with uncertain items visibly marked.
3. Ask the user to correct owners, due dates, wording and classification before
   treating anything as confirmed.
4. Return the confirmed proposal and the items that still need clarification.

## Completion

This is an advisory orchestration skill. A future governed task or decision
writer must receive only confirmed items, use its own confirmation and receipt
contract, and report unsupported persistence honestly.
