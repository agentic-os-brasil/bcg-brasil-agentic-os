---
name: meeting-to-work-items
description: Turns a meeting transcript or notes into structured decisions, action items, and follow-ups — and files each into its proper atlas home. Use when the user shares a transcript or meeting notes and wants "what came out of this" captured, not lost in chat.
---

# Meeting to Work Items

An orchestration ritual: it extracts structured items from a transcript and routes each to
the atomic capability that owns it. It composes — it never re-implements task creation or
decision logging.

## Workflow
1. If the transcript is long, call **`briefing-analyst`** to parse it into a clean digest.
2. Extract three kinds of item: **decisions**, **action items**, **follow-ups**.
3. File each via the owning capability:
   - action items → **`task`** (creates them on the external tool + backlog);
   - decisions → **`decision-log`** (into the relevant project's Decisions subsection);
   - durable client facts → **`client-keeper`**; project facts → **`work-logger`**.
4. Note any risk/urgent item in the daily log and flag it.
5. Return a scannable digest: decisions · actions (with owners) · follow-ups · risks.

## Relations
- **Orchestration**, held by the hub. Calls `briefing-analyst`, `task`, `decision-log`,
  `client-keeper`, `work-logger`. Distinct from `task`: `task` is a single manual op; this
  is bulk extraction that *uses* `task` for the action-item step.
