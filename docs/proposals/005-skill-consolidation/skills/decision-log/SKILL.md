---
name: decision-log
description: Records a durable decision into the relevant project's Decisions subsection — a four-letter permanent code, date, decision, context, review-by date, and status — append-only, never rewriting a past entry. Use when a lasting choice is made (methodology, scope, commercial terms) that must never be re-litigated or re-discovered.
---

# Decision Log

A durable decision is recorded once, in one place, and never silently changed. Executed by
`work-logger`; this skill defines the method so it's consistent wherever a decision surfaces.

## Method
1. Confirm the choice is durable (a methodology/scope/stakeholder/commercial call), not a
   task or status — those go elsewhere.
2. Choose a memorable four-letter uppercase code; verify it isn't already used.
3. Append an entry to the project's Decisions subsection: code · date · decision · context /
   source · review-by date · status (`active`). Never edit a past entry's substance.
4. To change a prior decision, append a NEW entry and flip the old one's status to
   `superseded by <CODE>` — that is the only edit allowed on a past entry.

## Relations
- **Executed by `work-logger`** (owner of the project file and its Decisions subsection).
- **Called by** `eod` (reconcile the day's decisions) and `meeting-to-work-items` (decisions
  surfaced in a transcript).
