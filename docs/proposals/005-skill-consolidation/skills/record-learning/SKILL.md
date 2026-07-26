---
name: record-learning
description: Records a durable, career-spanning learning into the learnings log — a realization about doing the work well, leading, handling pressure, or navigating a career that stays true beyond any single project. Use when a standout insight surfaces mid-work ("I learned that…", "lesson from this", "note for the future"), or when the weekly retro distills one.
---

# Record Learning

A learning is durable wisdom — not a case decision (`decision-log`) or a technical method
(`record-concept`). Executed by `career-keeper`.

## Method
1. Confirm it's a durable, career-spanning learning: it would still make sense years from
   now, without the specific case as context. If it's case-specific, it's a decision; if
   it's a reusable technique, it's a concept.
2. Append one entry to `learnings/learnings-log.md`, newest first: date · what happened
   (the moment that surfaced it) · the learning (stated to outlive the case) · related
   (a project / person / concept, if any).
3. Never rewrite a past entry. If a later experience complicates an old learning, add a new
   entry noting the tension — the contradiction is itself part of the record.

## Relations
- **Executed by `career-keeper`** (owner of `learnings/`).
- **Called by `retro`** — its WRITE step promotes the week's durable learnings through this
  skill — and invocable directly the moment an insight surfaces, from any flow.
