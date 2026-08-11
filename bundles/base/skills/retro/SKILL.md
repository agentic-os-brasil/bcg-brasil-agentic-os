---
name: retro
description: Run the weekly professional retrospective against the owner's development objectives, then record it in the owner atlas. Use at the end of a working week, for "retro", "retrospectiva", "let's close the week", "how did this week go against my objectives", or when a standing grant wakes the weekly ritual.
---

# Retro

Walk the week with the owner, then write what they decided. This is a
conversation that produces a page, not a report generator.

All reads and writes go through the owner atlas operations exposed by the
installed runtime adapter (`bcgos atlas owner collect`, `create-page`,
`append-entry`). Never edit an atlas page directly from this skill.

## Interaction profile

Resolve `interaction-profile` before presenting anything. The reads, the
writes, the bounds and the confirmation behaviour never vary by profile; only
the explanation and optional detail do.

- `standard`: the walk, the evidence found, one intention for next week.
- `advanced`: add which objectives had no evidence and why that may be, plus
  what was carried over from the previous retrospective.
- `power`: add the page revisions read, the idempotency key used for each
  write, and the authority the write ran under.

## Inputs

Obtained with `collect`, always with a declared purpose and named pages. There
is no whole-root read.

- this week's pages in `owner/daily/`, if the owner keeps them;
- current objectives from `owner/development/objectives.md`;
- the previous one or two retrospectives in `owner/development/retros/`.

If a page is absent, `collect` reports it as an omission. Say so and continue —
a first retrospective has no predecessor, and that is not an error.

## Workflow

1. Resolve the week being closed and the page path for it,
   `owner/development/retros/<YYYY-MM-DD>.md`.
2. `collect` the inputs above. Keep the revision of each page read: a later
   write uses it to detect that the owner edited the page in the meantime.
3. Walk the week **as a conversation**, not as a summary handed over:
   - what went well, and what did not;
   - per objective, where it showed up this week and where it was missed;
   - bring specific evidence from the daily pages rather than impressions, and
     ask before concluding. If the evidence is thin, say it is thin.
4. Name any pattern that repeats against the previous retrospectives. A pattern
   across weeks is worth more than any single week's detail.
5. Land on **one** intention for next week: concrete and observable, so the
   next retrospective can tell whether it happened.
6. Write the retrospective page with `create-page`. An existing page for the
   same week is preserved — offer to append to it instead of replacing it.
7. Offer to add each strong piece of evidence to the objective it belongs to,
   with `append-entry` under that objective's evidence section. Confirm each
   one separately; nothing enters an objective without the owner agreeing to it.
   If a page declares the same evidence heading more than once, the operation
   refuses the write rather than guessing which objective was meant — name the
   objective's own heading instead.
8. If the week produced a durable claim about how this kind of work goes — not
   what happened, but what it suggests is generally true — offer to promote it
   to `owner/learnings/` with `create-page`, one page per claim. State it as a
   generalization: the engagement may be named as the source, but findings,
   figures and client material stay in the workspace that owns them.
9. Report what was written, what was proposed but not written, and anything the
   owner declined.

Promotion in step 8 is **attended only**. A standing grant covers one page
family, so an occurrence woken under the weekly retro grant can write the
retrospective but not a learning. That is the right shape rather than a
limitation to route around: a durable claim about the owner's profession is
theirs to make, not something a scheduled job should decide on their behalf.

## Register

This ritual is reflective, and the tone should be warmer and slower than a
planning ritual. That is a matter of register only. It widens no bound, skips
no confirmation and does not soften an honest reading of the week.

## Invariants

- The skill never writes a file. Every effect is a named operation through the
  installed adapter.
- Nothing is recorded that the owner did not agree to in the conversation.
- Evidence is quoted from what the owner wrote, never invented to fill an
  objective that had a quiet week.
- Client and engagement content stays in the workspace that owns it. A
  retrospective may name the engagement worked on; it does not copy findings,
  figures or deliverable material into owner scope.
- A write that reports `proposed` rather than `written` means the page changed
  underneath the read and nothing was persisted. Show the owner the proposal
  and let them decide; do not retry over their edit.
- If an operation is unavailable, say so and continue the conversation. The
  walk, the reflection and a reviewable draft are all still worth having — only
  the recording is lost, and it must not be reported as done.
