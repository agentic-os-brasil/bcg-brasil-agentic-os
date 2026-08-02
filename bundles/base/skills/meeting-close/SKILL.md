---
name: meeting-close
description: Produce a reviewable closure packet from supplied meeting notes by structuring work items, decisions, unresolved follow-ups and next actions.
---

# Meeting Close

Use when a Case Agent needs a reliable end-of-meeting checkpoint. This is a
bounded composition of `meeting-to-work-items`; it stops before external
systems, messages or durable memory.

## Interaction profile

Resolve the canonical `interaction-profile` before presenting the packet. It
changes explanation depth only; it never enables persistence, integrations or
delegation.

## Required input

- meeting notes explicitly scoped to a workspace;
- meeting title/date when known;
- owning Case Agent and intended downstream destination, if any.

Missing notes, workspace scope or downstream authorization returns
`unavailable/input_scope`. The skill may still return a partial extraction
when the supplied notes are sufficient, with gaps made explicit.

## Method

1. Run the pure `meeting-to-work-items` transformation on the supplied notes.
2. Separate resolved decisions, owned tasks, unassigned follow-ups and
   participants.
3. Build a closure summary: what changed, what remains open, dependencies and
   the next safe action.
4. Flag items needing human confirmation before any task, message, calendar,
   memory or workspace write.
5. If the meeting has material governance, scope or commitment implications,
   mark the packet for Walter review. Do not claim that review occurred.

## Output contract

Return `meeting_summary`, `decisions`, `tasks`, `follow_ups`, `participants`,
`confirmation_required`, `review_required`, `proposed_next_actions` and
`unavailable_checks`. Include source pointers for extracted items when
available. `persistence_status` is always `not_attempted` for this skill.

## Invariants

- No Notion, CRM, email, calendar, browser, file write or memory commit.
- No client/account promotion and no stakeholder update from participant names.
- No hidden child-agent call: Maestro remains the hub; Case Agent owns the
  context; a PA Expert consultation requires an explicit bounded packet.
- If an integration is unavailable, preserve the reviewable packet and report
  the capability as `unavailable` rather than emulating it.
