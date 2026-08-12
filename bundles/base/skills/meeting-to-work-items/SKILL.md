---
name: meeting-to-work-items
description: Extract decisions, tasks, follow-ups and participants from supplied professional meeting notes without creating tasks or persisting content. Use for "what came out of this meeting", "pull the action items", "list the decisions", or right after a call closes.
---

# Meeting to Work Items

Use for a bounded transformation of supplied meeting notes. It can run
standalone or provide the structured input for `meeting-close`.

## Interaction profile

Resolve the canonical `interaction-profile` before presenting the result. The
profile changes explanation depth only; extraction rules and data scope stay
fixed.

## Required input

`meeting_note` is required. `meeting_title`, `meeting_date`, `workspace_id` and
`scope_hint` are optional but should be supplied by the Case Agent when known.
The note must be explicitly in scope for the current workspace. If it is
missing, too short to support a reliable extraction or outside the authorized
scope, return `unavailable/input_scope` and do not invent items.

## Method

1. Identify the title and date, preserving uncertainty when they are not
   explicit.
2. Extract only decisions stated as resolved, not debates or hypotheses.
3. Extract concrete tasks in imperative form. Set `owner`, `due`, `project`
   and `priority` only when explicit or supported by a stated rule.
4. Extract unresolved follow-ups separately; mark missing owners as
   `unassigned`, never guess.
5. Extract participants only when named in the supplied note.
6. Attach an evidence pointer or short source span to every non-empty item
   when the input format permits it.

## Output contract

Return JSON-serializable data with `meeting_title`, `meeting_date`,
`decisions`, `tasks`, `follow_ups`, `participants` and `unavailable_checks`.
Each decision contains `statement` and `evidence`. Each task contains `name`,
`owner`, `due`, `priority` and `evidence`. Each follow-up contains
`description`, `owner`, `due` and `evidence`.

## Invariants

- Never call Notion, a task system, a calendar, a browser or another agent.
- Never convert an ambiguous statement into a decision or an owned task.
- Never persist raw notes, names or client content in the managed bundle,
  telemetry or memory.
- If a downstream creator or capability is unavailable, return the packet and
  mark the downstream action `unavailable`; do not perform a substitute write.
