---
name: extract-work-items
description: Extract proposed decisions, action items, follow-ups, owners and risks from user-supplied meeting notes without creating tasks or writing records. Use for “what came out of this meeting?”, “extract actions from these notes” or “turn this transcript into work items”.
---

# Extract Work Items

Resolve the canonical `interaction-profile` before responding. It changes
explanation depth only; it never authorizes transcript retrieval or writes.

## Contract

- Accept only notes or a transcript supplied in the current request.
- Return four clearly labeled proposal lists: decisions, action items,
  follow-ups and risks.
- For every item, preserve the supporting note fragment or mark its evidence as
  unclear; never infer an owner, deadline, decision or commitment.
- Do not read a calendar, task system, workspace, email or prior meeting; do
  not create a task, decision record, memory item or external message.

## Method

1. Separate an explicit decision from a discussion or preference.
2. Separate an action from an aspiration; state owner and due date only when
   the supplied notes say them.
3. Surface unresolved questions as follow-ups, not completed work.
4. Flag ambiguity, conflict and urgency for human review.

## Completion

Return proposed work items only. Use `meeting-to-work-items` when the user
wants the review conversation; an authorized future writer may persist only
items the user confirms.
