---
name: task
description: Normalize a user-supplied task request into a proposed task with scope, owner, due-date confidence and completion criteria without creating or changing any task. Use for “add a task”, “mark this done”, “move this task” or “what tasks are implied here?”.
---

# Task

Resolve the canonical `interaction-profile` before responding. It changes
presentation only; it never grants task-system, backlog or workspace authority.

## Contract

- Accept only a task request or confirmed proposal supplied in the current
  request.
- Return proposed operation, title, owner, due-date confidence, dependencies
  and completion criteria.
- Ask for clarification when an operation could create duplication or change a
  task's owner or deadline.
- Do not list, create, complete, move or synchronize external/local tasks.

## Completion

Return a task proposal. A future task provider must confirm, use an idempotency
key, issue metadata-only receipts and recover partial failures.
