---
name: start-day
description: Starts or resumes the workday at whatever hour the user shows up — reads the atlas, pulls email and calendar, and returns one briefing scoped to what has already happened and what's still left today. Use at first contact of the day, "good morning", "what's my day look like", or a re-entry later the same day.
---

# Start Day

An orchestration ritual: it composes a briefing from several agents and synthesizes one
crisp message. It adapts to *when* it's actually run (morning vs. mid-afternoon vs. evening).

## Workflow
1. Get the current time; check whether today's daily log already has content (first contact
   vs. re-entry). Read the atlas: owner profile, backlog, the 2 most recent daily logs, and
   any active client/project files they point to.
2. Call **`briefing-analyst`** for today's full calendar (so past vs. upcoming can be split
   by the current time) + important email.
3. Compute the work hours actually left today (workday end − now − any protected block).
4. Call **`work-planner`** (CAREER mode, short-term horizon) for a ranked plan scoped to the
   hours that remain.
5. Synthesize ONE briefing: today's shape (past marked done, upcoming highlighted) · top 3
   for the time left (each with a one-line why) · watch-outs · a development nudge tied to
   something still ahead · suggested first move. For a re-entry, lead with a short "so far
   today" recap instead.
6. Record the plan in today's daily log via **`work-logger`**: if today's log doesn't exist
   yet, it creates it from the template (and adds it to the daily index); if it already
   exists, it appends a timestamped entry — never overwrites.

## Relations
- **Orchestration**, held by the hub. Calls `briefing-analyst` + `work-planner`; writes via
  `work-logger`. If remaining hours are near zero, it offers `eod` instead of forcing a plan.
