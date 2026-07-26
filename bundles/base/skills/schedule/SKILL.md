---
name: schedule
description: Propose meeting slots from availability explicitly supplied in the current request without reading calendars or creating events. Use for “schedule this”, “find a time”, “suggest slots” or “prepare a meeting invite”.
---

# Schedule

Resolve the canonical `interaction-profile` before responding. It changes
presentation only; it never grants calendar, contact or event authority.

## Contract

- Accept only participants, duration, time zone, window and availability the
  user supplies now.
- Return 1–3 proposed slots with assumptions and a draft event description.
- Ask for missing time zone, duration or participant constraints.
- Do not inspect conflicts, read calendars, create an event, notify invitees or
claim a booking succeeded.

## Completion

Return proposals only. A future calendar capability needs explicit confirmation,
idempotency, receipt and rollback before it may mutate an event.
