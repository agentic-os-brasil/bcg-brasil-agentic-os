---
name: eod
description: Close the working day on the owner's daily page — what got done, what moved, what carries to tomorrow — and surface anything that deserves a durable home elsewhere. Use for "eod", "fechando o dia", "wrapping up", "done for today", or when the owner is stopping.
---

# End of Day

Close the day honestly and leave tomorrow a starting point.

All reads and writes use direct file operations on the owner atlas paths (`data/owner/atlas/`). Never skip the confirmation gate or edit atlas files directly outside the skill's write sequence.

## Interaction profile

Resolve `interaction-profile` before presenting. The reads, the write and the
bounds never vary by profile.

- `standard`: the three-line recap and tomorrow's first priority.
- `advanced`: add what was surfaced for a durable home and what the owner
  declined to record.
- `power`: add the page revision, the idempotency key and the authority the
  write ran under.

## Inputs

Obtained with `collect`: today's page in `owner/daily/`, and current objectives
from `owner/development/objectives.md` when the day touched one.

## Workflow

1. Ask what actually happened, rather than summarizing what was logged earlier.
   The morning plan is a hypothesis; the point of closing is to record the day
   that occurred.
2. Record on today's page: what got done, what moved and did not finish, and
   what carries to tomorrow. `create-page` if the day has no page yet, then
   `append-entry`.
3. **Surface durable decisions, do not file them.** If the day produced a
   decision that outlives it, say so and name where it belongs. A decision
   about an engagement belongs to that workspace, and this skill does not reach
   across scope to write it.
4. Note evidence bearing on a development objective, good or missed. Offer to
   append it under that objective's own heading, confirming each separately. If
   the objectives page repeats an evidence heading, name the objective's
   heading — the operation refuses an ambiguous target rather than guessing.
5. If the day produced a durable claim about how this kind of work goes, say so
   and offer to promote it. Promotion is a separate, attended act.
6. Preview tomorrow's first priority in one line.
7. Give a three-line recap: what closed, what carries, what is first tomorrow.

## What this skill narrowed

An earlier design reconciled an external task system and a project's current
truth as part of closing. It does neither.

Where a workspace fact changed today, the day's page notes that the workspace
page needs updating. It does not reach across scope to update it: workspace
content belongs to the workspace that owns it, and closing the owner's day is
not authority over an engagement's record.

## Invariants

- Append-only per day. Closing twice adds a second timestamped entry rather
  than overwriting the first; the two attended check-ins remain auditable.
- Nothing is recorded that the owner did not confirm in the conversation.
- An engagement may be named. Findings, figures and deliverable material stay
  in the workspace that owns them.
- A result of `proposed` rather than `written` means the page moved under the
  read. Show the owner the proposal; do not retry over their edit.
- If an operation is unavailable, close the day in conversation and say the
  recording did not happen. A day discussed and not written is an honest
  outcome; a day reported as written when it was not is a lie the owner will
  discover later.
