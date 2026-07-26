---
name: eod
description: Review user-supplied work from today, identify carry-forward items and prepare a concise next-day starting point without writing logs or tasks. Use for “eod”, “wrap up my day”, “close today” or “what carries over?”.
---

# End of Day

Resolve the canonical `interaction-profile` before responding. It controls
explanation depth only; it never authorizes task, memory, calendar or workspace
writes.

## Orchestration contract

- Accept only a current-request recap of completed work, open items and risks.
- Separate confirmed completion from proposed carry-forward, decision and task
  candidates.
- Return a concise recap, unresolved risks and one recommended first move for
  the next work period.
- Do not read a daily log, reconcile an external task system, record a decision
  or claim that any item was persisted.

## Workflow

1. Classify supplied items as done, blocked, carried or unclear.
2. Use `extract-work-items` only when the recap contains a conversation that
   needs decisions/actions proposed for review.
3. Keep the carry-forward list short and surface dependencies or missing owners.
4. Ask for confirmation before treating a future action as committed.

## Completion

Return an advisory closeout. A future writer may materialize confirmed items
only through its separate task, decision or execution contract.
