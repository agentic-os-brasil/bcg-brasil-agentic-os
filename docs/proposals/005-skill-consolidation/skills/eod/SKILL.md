---
name: eod
description: Closes the day — finalizes today's log, reconciles decisions and load-bearing facts, syncs tasks, and previews tomorrow's first priority. Use for "eod", "wrapping up", "done for today". On Fridays it also offers the weekly retro and a memory-hygiene pass.
---

# End of Day

An orchestration ritual that leaves the atlas clean and tomorrow set up.

## Workflow
1. Call **`work-logger`** to finalize today's daily log (done / decisions / carried to
   tomorrow), update the backlog (check off, re-rank, archive), and flip any changed project
   status.
2. Reconcile the single-source homes: durable decisions made today go through
   **`decision-log`**; any load-bearing number that changed is updated in its project's
   Current-truth block (via `work-logger`).
3. Reconcile tasks with the external tool via **`task`** (mark done on both sides, add
   anything one-sided).
4. If client/people facts surfaced today, ensure the right keeper captured them.
5. Note any development-objective evidence in the daily log (feeds `retro`).
6. Give a 3-line recap + tomorrow's likely first priority.
7. **On Fridays:** offer **`retro`** and **`consolidate`** if they haven't run this week.

## Relations
- **Orchestration**, held by the hub. Calls `work-logger`, `decision-log`, `task`; offers
  `retro` + `consolidate` on Fridays.
