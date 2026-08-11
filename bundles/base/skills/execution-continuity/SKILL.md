---
name: execution-continuity
description: Register, pause and resume bounded professional work with local checkpoints across sessions.
---

# Execution Continuity

Use this skill when the owner asks to register a task, continue open work,
pause a deliverable, create a checkpoint or resume work from a previous
session. This is the runtime-neutral execution ledger; it is not the owner
`work-state.md`, a transcript, a task provider or memory.

## Resolve the Maestro CLI

Use the exact executable path emitted by SessionStart. Never assume that
`bcgos` is on the desktop runtime's `PATH`. If no executable path is
available, explain that the local core could not be found and wait for the
installer or runtime to be reopened. Never substitute a different binary.

Resolve the canonical `interaction-profile` before presenting the task
summary or checkpoint. It controls communication style only; it does not
choose the task, grant authority or replace owner confirmation.

## Register a new task

1. Confirm the workspace and summarize the objective, the next smallest step
   and one bounded completion criterion. Do not create a task merely because a
   phrase appeared in a conversation; ask for one short confirmation when the
   owner has not explicitly asked to track it.
2. Read the accepted contract with `<maestro-cli> work schema`.
3. Create the item with `work create --workspace <workspace> --stdin` using
   only the objective, initial next step, allowed logical references and
   bounded criteria. Do not place prompts, transcripts, client bodies,
   credentials or absolute paths in the contract.
4. Start the created item with the returned item ID and revision. Report the
   task as registered only after both writes succeed.

The execution ledger is separate from `owner/operating/work-state.md`.
`ownerctx` open tasks may remain empty while a local execution item is active;
that is an intentional authority boundary, not a persistence failure.

## Pause and checkpoint

When the owner pauses, changes the next action or reaches a session boundary,
write a concise checkpoint through `work checkpoint --stdin`, then pause the
item. A checkpoint may contain only a bounded summary, next step, blocker and
logical artifact references. Never synthesize one from a transcript, prompt,
tool payload or Stop hook without an explicit owner-approved next step.

## Resume in a new session

At SessionStart, inspect the bounded status first. If `open_work` is available
and a checkpoint is available, resolve the pointer explicitly with
`<maestro-cli> work next --active --workspace <workspace>`.

Confirm the scope and next step with the owner, then use `work resume` to begin
the new fenced attempt. If there is no active item, say so and offer to create
one. If multiple items are active, require an explicit item ID. Never treat
`native_qualified:false` as a reason to refuse local ledger operations;
qualification is separate runtime evidence. Do not claim continuity until the
new runtime session has actually observed the bounded projection.

## Truth and safety

- `work list` is metadata-only; use `work inspect` or `work next` only when the
  owner has authorized reading the bounded work handoff.
- Preserve the current workspace identity and reject a path or workspace ID
  that does not match the SessionStart context.
- Keep external task systems, SharePoint, memory and agent delegation outside
  this skill unless their own governed capability is explicitly available.
- An unavailable native qualification label does not make the local execution
  ledger unavailable.
