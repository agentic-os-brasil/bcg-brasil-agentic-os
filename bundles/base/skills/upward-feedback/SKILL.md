---
name: upward-feedback
description: Help the owner prepare considered feedback to give upward to a senior colleague, producing the owner's own preparation in owner scope rather than a record about that colleague. Use for "help me prepare feedback for my project leader", "draft my upward feedback", "I have to fill in the feedback form for my manager", or "what should I say about how the case ran".
---

# Upward Feedback

Help the owner work out what to say, and how to say it, before they say it.

All reads and writes use direct file operations on the owner atlas paths (`data/owner/atlas/`). Never skip the confirmation gate or edit atlas files directly outside the skill's write sequence.

## The boundary that defines this skill

**The artifact is the owner's own preparation, held in owner scope. It is not a
record about the other person.**

Proposal 003 deferred cross-project people records entirely. A page that
accumulates observations about a named colleague is that record under another
name, and this skill must not create one. The line is the subject of the
sentence:

| Admitted | Denied |
| --- | --- |
| What the owner observed, at a moment they can point to | An inferred trait, a disposition, a tendency |
| What the owner concluded from it | A behavioural score, rating or ranking |
| What the owner intends to say, and how | A standing profile of the colleague |
| The effect on the owner's own work | Any content whose subject is the colleague rather than the owner's own message |

The colleague is necessarily named — the owner is preparing to speak to a
specific person. Naming is not the risk; accumulating is. So:

- **one preparation, one page.** Preparations are never merged into a running
  record, and no prior preparation about the same person is retrieved to seed a
  new one. The owner starts from what they think now.
- **the file is named for the occasion, not the person.** No segment proposal
  declares a page kind for this, so write where the owner names it, defaulting
  to `owner/development/upward-feedback/<YYYY-MM-DD>-<occasion-slug>.md`. A
  per-person filename would turn pages that are individually fine into the
  dossier this boundary exists to prevent.
- **the page says what it is.** State on it, in the owner's words, that it is
  their preparation for one conversation.

## Interaction profile

Resolve `interaction-profile` before presenting a draft. The boundary above,
the operations used, the bounds and the confirmation behaviour never vary by
profile; only the explanation and optional detail do.

- `standard`: the draft, and what the owner still has to decide.
- `advanced`: add why each point is framed the way it is, and what register the
  seniority difference calls for.
- `power`: add the pages collected and their revisions, the operation behind
  each write, and every point dropped for lacking an anchor the owner
  confirmed.

## Inputs

- The owner's own account of what happened, in session. This is the primary
  source and it outranks anything retrieved.
- Obtained with `collect`, purpose declared and pages named: the owner's daily
  pages covering the period, and any retrospective that touches it. Read them
  for concrete moments the owner can confirm — never to assemble a
  characterisation of the colleague. There is no whole-root read and no folder
  listing, so name the days. An absent page is an omission, not an error.

## Workflow

1. **Ask the owner** who the feedback is for, their seniority relative to the
   owner, and the period it covers. Never look any of it up: a directory lookup
   is how a preparation page starts becoming the colleague record this skill
   exists to prevent. Seniority calibrates register, not candour.
2. Ask the owner what they already want to say, and start there. They usually
   know; the work is sharpening it, not sourcing it.
3. Offer concrete moments from the owner's own pages as anchors and ask them to
   confirm each one. An unconfirmed moment is dropped, not softened.
4. Draft in behaviour and effect, never in adjectives about the person: what
   happened, and what it caused for the work or the team. Frame anything the
   owner wants changed as a practice they are asking for, not a deficiency they
   have diagnosed.
5. Where the owner has nothing genuine for a section, say so. A named gap beats
   a manufactured point, and a form does not have to be filled to be answered
   honestly.
6. Iterate turn by turn on their corrections. Do not re-ask what they have
   already answered.
7. On confirmation, `create-page` for this one preparation. A page already at
   that path is preserved and reported `unchanged`, never replaced — if the
   owner is preparing a second conversation, that is a second page.
8. Further passes on the same preparation are `append-entry` under a heading
   the page already declares. `append-entry` never creates a heading, and it
   refuses a heading that appears more than once on a page — name the section's
   own heading rather than a repeated one, or hand the text back as a draft to
   place.
9. Close by reporting whether the page was written or came back as a proposal,
   then restating that the page is preparation, that delivering the
   feedback is the owner's act alone, and what is left for them to decide.

Every run is attended. Nothing here is scheduled, and no standing grant reaches
this page family.

## Invariants

- The skill never writes a file. Every effect is a named operation through the
  installed adapter.
- One preparation, one page. Nothing is merged into a per-person record, and no
  prior preparation about the same person is read to start a new one.
- No inferred trait, behavioural score or standing profile is produced, and
  nothing is recorded whose subject is the colleague rather than the owner's
  own message.
- Every point traces to a moment the owner confirmed. No moment is invented,
  and none is retained from a page the owner did not confirm.
- Delivery is the owner's act. This skill prepares; it sends nothing, submits
  nothing and discloses nothing outside owner scope.
- Client and engagement content stays in the workspace that owns it. The
  preparation may name the engagement it concerns; findings, figures and
  deliverable material do not move into owner scope.
- Content from another workspace or another owner is never admitted, and
  nothing here is read into or written out of any memory layer.
- A write that reports `proposed` rather than `written` means the page moved
  underneath the read and nothing was persisted. Show the owner the proposal
  and let them decide; do not retry over their edit.
- If an operation is unavailable, say so and keep going. The thinking, the
  framing and a draft the owner can take to the conversation are the point —
  only the recording is lost, and it must never be reported as done.
