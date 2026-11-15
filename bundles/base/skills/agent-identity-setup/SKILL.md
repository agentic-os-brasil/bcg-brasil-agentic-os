---
name: agent-identity-setup
description: Personalize the main agents' names and emoji through the canonical Maestro MCP identity interview, or resume its pending draft. Use when the owner chooses agent presentation during onboarding or explicitly asks to rename an agent. Changes presentation only, never runtime authority.
---

# Agent identity setup

Use the admitted Maestro connector for all identity operations. Names and
emoji belong to the canonical private identity profile; never write legacy
`agents.json`, per-case `agent.json` or managed templates yourself.

## Open the current question

1. Query `agent.identity.interview`. Follow its actual `state`,
   `profile_revision`, `next_question` and `open_draft_id`. Reading never
   initializes a profile. A blocked/invalid state stops this interview;
   preserve it rather than treating it as a clean installation.
2. For `action_required`, ask only `next_question`. Use its role explanation,
   default name, emoji and transparent suggestions from the catalog. The
   canonical order is Maestro, the internal reviewer, then system health.
   Do not batch a future answer or infer psychological preferences.
3. Carry the existing profile's `owner_id` forward. For a first profile,
   agree one stable nonsensitive local owner identifier. Do not request
   credentials. Personalization is optional; absent preferences leave the
   installed defaults active and do not block unrelated professional work.

## Save one answer

Apply `agent.identity.answer` with only the owner-provided `display_name`
and `emoji`, the current `role` and `question_id`, `owner_id`, and the exact
`expected_profile_revision` just returned. The host presents complete current
and proposed preferences plus the owner-only/no-client-data attestation.
One native review creates and confirms this answer through the canonical
draft lifecycle. Never send consent, approval, scope or review-digest fields.

After an applied receipt, query the interview again. Verify the confirmed
name/emoji and then ask its next question. `complete` means the three main
questions are complete; it does not activate agents, grant tools or qualify
providers. An explicit later rename uses its canonical `identity-<role>`
question ID and a freshly read profile revision, preserving all other choices.

For a deliberate pause before applying, use `agent.identity.propose` instead.
Its native review authorizes an inactive canonical draft only. Report the
returned proposal ID and `review_required`; the confirmed profile is unchanged.

## Resume a pending draft

For `review_required`, query `agent.identity.review` with exactly the returned
`proposal_id`, then apply `agent.identity.apply` with that same ID. Metadata
queries do not expose or approve the proposed body; the native host presents
its full current/proposed profile for confirmation. Do not ask another
question or create a competing draft.

Declining an ordinary answer creates no draft or profile. Declining an
existing draft keeps that inactive review pending; report this and stop rather
than deleting it or advancing the interview. A post-review I/O error may leave
an open draft or an already-applied profile: use the returned recovery metadata
and query again. Repeating an applied draft returns its current exact receipt;
a stale answer or later profile change requires fresh review.

## Preserve boundaries

- Unknown/mismatched role, emoji, owner, question or revision is refused.
- An account/case/PA identity requires a real admitted instance and explicit
  `agent_id`. Missing registration is pending; never substitute a global role.
  Case instances must belong to this admitted workspace. Keep signed instance
  definitions and runtime state unchanged.
- `confirmation_unavailable` or a denied native form authorizes no alternate
  write. Explain the client limitation; do not invoke a shell, raw filesystem,
  another connector or an installation operator to persist preferences.
- Personalization remains owner data. Never place names/preferences into client
  context, provider packets, telemetry or the managed product.
