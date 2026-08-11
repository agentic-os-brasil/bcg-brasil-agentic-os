---
name: execution-continuity
description: Register, pause and resume bounded professional work with local checkpoints across sessions.
---

# Execution Continuity

Use this skill when the owner asks to register a task, continue open work,
pause a deliverable, create a checkpoint or resume work from a previous
session. This is the runtime-neutral execution ledger; it is not the owner
`work-state.md`, a transcript, a task provider or memory.

## Runtime-first continuity

Use the active workspace and its readable artifacts as the source of truth.
Do not expose `bcgos`, run IDs, revisions or JSON contracts to the owner. The
legacy deterministic execution ledger remains a compatibility/recovery surface
for older workspaces, but a new task is represented first by the agent in the
canonical workspace locations.

Resolve the canonical `interaction-profile` before presenting the task summary
or checkpoint. It controls communication style only; it does not choose the
task, grant authority or replace owner confirmation.

## Register a new task

1. Confirm the workspace and summarize the objective, next smallest step and
   one bounded completion criterion. When the owner explicitly asks to track
   the work, create it directly; ask only if the workspace, scope or next
   action is genuinely ambiguous.
2. Write a concise Markdown task under `brain/tasks/` and
   link it to the relevant artifact under `brain/projects/` or
   `brain/deliverables/`. Include only objective, next step, owner,
   completion criterion and logical artifact references.
3. Do not place prompts, transcripts, client bodies, credentials or absolute
   paths in the task file. Report the task as registered after the reviewed
   artifact is written.

The execution ledger is separate from `owner/operating/work-state.md`.
`ownerctx` open tasks may remain empty while a local execution item is active;
that is an intentional authority boundary, not a persistence failure.

## Pause and checkpoint

When the owner pauses, changes the next action or reaches a session boundary,
append a concise `## Checkpoint` section to the reviewed task or deliverable.
It may contain only a bounded summary, next step, blocker and logical artifact
references. Never synthesize one from a transcript, prompt, tool payload or
Stop hook without an explicit owner-approved next step.

## Resume in a new session

At SessionStart, inspect the active workspace and its bounded task/checkpoint
artifacts first. Confirm the scope and next step with the owner, then continue
from the most recent reviewed checkpoint. If there is no active item, say so
and offer to create one. If multiple items are active, ask which one to use.
Do not claim continuity until the new runtime session has actually observed the
bounded projection.

## Truth and safety

- `work list` is metadata-only; use `work inspect` or `work next` only when the
  owner has authorized reading the bounded work handoff.
- Preserve the current workspace identity and reject a path or workspace ID
  that does not match the SessionStart context.
- Keep external task systems, SharePoint, memory and agent delegation outside
  this skill unless their own governed capability is explicitly available.
- An unavailable native qualification label does not make the local execution
  ledger unavailable.
