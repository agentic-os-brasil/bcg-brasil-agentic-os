---
name: start-day
description: Turn user-supplied commitments, available time and priorities into a focused workday plan. Use for “start my day”, “what should I focus on?”, “plan today” or a mid-day reset.
---

# Start Day

Resolve the canonical `interaction-profile` before responding. It controls
explanation depth only; it never authorizes calendar, email, task or workspace
access.

## Orchestration contract

- Accept only commitments, availability, priorities and concerns supplied in
  the current request.
- Compose `wayfinder` when the priority is unclear; otherwise rank the work
  directly.
- Return 1–3 priorities, time allocation, watch-outs and one first move.
- Do not read calendar/email/task systems, create a daily log, infer workload
  from prior sessions or claim the plan was saved.

## Workflow

1. Confirm the time window and any non-negotiable commitments.
2. Separate urgent commitments from the decision that matters most today.
3. Rank a deliberately short plan against the time available.
4. Make overload visible and identify what should not be attempted.

## Completion

Return a current-request advisory plan. Calendar, mail, task reconciliation and
durable daily-log updates remain unavailable until their governed capabilities
exist.
