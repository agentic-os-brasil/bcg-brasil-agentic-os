---
name: task
description: Quick operations on the user's external to-do list — add, complete, move, or list tasks — kept mirrored with the atlas backlog. Use for "add a task", "mark X done", "what's on my list", "move this to next week". An atomic task op, not the transcript-extraction ritual (that's meeting-to-work-items).
---

# Task

Atomic CRUD on the external task tool, mirrored to the backlog so the two never drift.
Executed by `work-logger` (which holds the task-management connector).

## Method
1. Resolve the operation: add / done / move / list.
2. Apply it on the external task tool via `MCP (task-management connector)`.
3. Mirror the change in the atlas backlog so both sides match.
4. Confirm in one line what changed.

## Relations
- **Executed by `work-logger`.**
- **Called by** `meeting-to-work-items` (to create the action items it extracts — it calls
  this skill rather than re-implementing task creation) and `eod` (backlog reconciliation).
