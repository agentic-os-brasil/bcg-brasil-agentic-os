---
name: schedule
description: Fast-lane calendar scheduling — checks conflicts and availability, proposes slots, and returns a prefilled calendar event for one-click confirmation. Use for "schedule X", "find a time for Y", "when am I free for a 1h block this week".
---

# Schedule

Do all the scheduling work so the user only confirms. Executed by `work-logger` (which
holds the calendar connector).

## Method
1. Parse the ask: who, how long, what window.
2. Check conflicts and find open slots via `MCP (calendar connector)`.
3. Return 1-3 concrete options, each as a prefilled event the user can confirm in one click.
4. On confirmation, log the booking to the relevant daily/project file.

## Relations
- **Executed by `work-logger`.** Reads owner working-preferences (protected blocks,
  deep-work windows) so it never proposes a slot that violates them.
