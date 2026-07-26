---
name: handoff
description: Compacts the current session into a structured handoff so a new session (or tomorrow) resumes cleanly. Use for "hand this off", "save context", "prep a resume", or when wrapping mid-task before context resets.
---

# Handoff

Operates on the hub's own working context — what the current session knows that a fresh
session wouldn't. This is why it stays with the hub rather than a sub-agent: only the hub
has the live session state.

## Method
Produce a structured block: **Goal** · **Current state** · **Decisions made** ·
**What remains** · **Dead ends tried** · **Open questions** · **Load-bearing facts (with
sources)**. Then promote anything durable to its proper atlas home (a project's Decisions
subsection, Current truth, a client/people file) so it survives beyond the handoff note.
Close with the 3 things a fresh session should read first.

## Relations
- **Hub-only** — compacts the hub's context. Durable items it surfaces are filed via the
  relevant keeper (`work-logger`, `client-keeper`) rather than left in the note.
