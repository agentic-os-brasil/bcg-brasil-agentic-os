---
name: maestro-onboarding
description: Guide or resume the owner's quick or complete professional interview through canonical owner status, one question at a time, reviewed proposals and trusted final confirmation. Use when the owner chooses onboarding, returns to an unfinished interview or reviews a completed profile.
---

# Maestro Onboarding

Establish a useful professional baseline through the admitted Maestro connector.
Canonical owner state determines progress; conversation history is supporting
context. This method grants no filesystem, shell, ingestion or scheduling authority.

## 1. Read progress before speaking

Call `maestro_query` with `operation: "owner.status"`. Use its `onboarding`
metadata, including `state`, `track`, `remaining`, `next_question` and
`review_digest`. Lifecycle metadata is a read-only hint; refresh status before
choosing a track, preparing an answer or confirming the profile.

| Status | Next action |
| --- | --- |
| `required`, `track: selection_required` | Offer the quick or complete track after the owner chooses to begin. |
| `required` or `in_progress`, with a selected track | Resume the returned `next_question`; preserve all applied answers. |
| `review_required` | Continue directly to final profile review in step 4. |
| `complete` | Continue the requested work. Review/refine only what the owner asks to change; never restart the interview automatically. |
| Uninitialized, unavailable or rejected read | Explain that the saved profile cannot be reached and continue useful work. Explicit installation/support owns preparation and recovery. |

Keep the opening warm, short and in the owner's chosen language. When no
language preference is known, ask in English which language to use and wait.
Use the [interaction-profile](../interaction-profile/SKILL.md) communication
guidance only when available through the admitted method resources; it grants
no owner-file access. Explain Maestro as a professional assistant that keeps
reviewed working context across conversations. Ask one question per turn.

If the owner defers, continue the requested task without repeating the offer in
that conversation. A deferral is not a completed profile or a persisted consent.

## 2. Select a track once

Offer `quick` (about 10 minutes) or `complete` (about 30 minutes). Quick covers
identity, authorized personal context, role, communication, preferences and
quality; complete adds the remaining professional facets. Personal disclosure
is optional: a reviewed statement choosing to keep that context private is a
valid answer. Do not infer identity, history or psychology.

After the owner chooses, call `maestro_apply` with `operation: "owner.track"`
and the chosen `track`. The trusted host review records the choice. Re-read
`owner.status` after a successful result. Selecting a track again, even the same
one, invalidates final confirmation; do it only for an explicit owner request.

Call `maestro_query` with `operation: "owner.interview"` and that exact `track`
to obtain its canonical steps and question wording. The returned step list is
the interview definition; use `owner.status` to determine which step is pending.
Do not add a second questionnaire, fixed question total or separate state file.

## 3. Review and save one answer

Ask the returned next question, then wait. A supported host question tool may
present concise choices with a free-text path; otherwise ask in plain language.
Accept out-of-order answers and re-read progress after applying each one.

1. Draft concise Markdown from the owner's explicit answer. Preserve their
   meaning and privacy choices. For a refinement, ask for a self-contained
   replacement or retain context the owner actually supplied; metadata does not
   reveal the existing private body.
2. Call `maestro_apply` with `operation: "owner.propose"`, the canonical `facet`,
   an `evidence` summary of the explicit answer and its `proposed_body`.
   A proposal remains pending until reviewed application succeeds.
3. Call `maestro_query` with `operation: "owner.review"` and the returned
   `proposal_id`. This returns metadata and `review_digest`, not private text.
4. Call `maestro_apply` with `operation: "owner.apply"` and that `proposal_id`.
   The trusted host review presents the exact current and proposed body,
   evidence, sensitivity, readers and update policy to the owner. The server
   binds approval to the current revision; model assertions cannot approve it.
5. On success, re-read `owner.status`. Resume its next pending question, or
   proceed to final review when its state becomes `review_required`.

Decline, cancellation, unavailable review or a rejected operation leaves the
profile unchanged. Do not retry a declined approval automatically. A revision
conflict requires a fresh status and review; preserve the later owner's work.

Owner operations neither import an existing brain nor read documents, public
URLs, prior chats or assessment files. If offered sources would help, explain
which separately available, explicitly authorized method could use them. Resume
this interview from owner-attested facts when that capability is unavailable.

## 4. Confirm the exact final profile

Call `maestro_query` with `operation: "owner.onboarding_review"` when all
required answers are applied. It returns track, state, facet count and the
current `review_digest`. This metadata is not the owner's review or approval.

Call `maestro_apply` with `operation: "owner.confirm_onboarding"` to present
the complete exact profile through trusted host review. The server obtains
human confirmation and revalidates its digest before recording completion.
Re-read `owner.status`; report completion only when it returns `complete`.
A declined or stale final review remains pending, with the saved answers intact.

If the host cannot provide trusted review, report the limit and leave the
pending state for an explicit supported operator review. This skill never runs
local commands, writes owner files or supplies confirmation flags as a fallback.

End with a brief plain-language result and continue the owner's work. Interview
completion establishes the reviewed profile only; connectors, agents, memory
consolidation, ingestion and recurring routines each retain their own capability,
consent and qualification requirements.
