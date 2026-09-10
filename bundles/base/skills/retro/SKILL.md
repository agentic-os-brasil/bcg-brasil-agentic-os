---
name: retro
description: Run the weekly professional retrospective against the owner's development objectives, then record it in the owner atlas. Use at the end of a working week, for "retro", "retrospectiva", "let's close the week", "how did this week go against my objectives", or when a standing grant wakes the weekly ritual.
---

# Retro

Walk the week with the owner, then write what they decided. This is a
conversation that produces a page, not a report generator.

All reads and writes go through the owner atlas operations exposed by the
installed runtime adapter (recuperar as páginas relevantes do atlas do owner, criar página,
registrar entrada). Never edit an atlas page directly from this skill.

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

Obtidos via recuperação do atlas, sempre com propósito declarado e páginas nomeadas. There
is no whole-root read.

- this week's pages in `owner/daily/`, if the owner keeps them;
- current objectives from `owner/development/objectives.md`;
- the previous one or two retrospectives in `owner/development/retros/`.

If a page is absent, `collect` reports it as an omission. Say so and continue —
a first retrospective has no predecessor, and that is not an error.

## Workflow

1. Resolve the week being closed and the page path for it,
   `owner/development/retros/<YYYY-MM-DD>.md`.
2. Recuperar as páginas relevantes do atlas do owner. Keep the revision of each page read: a later
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
6. Write the retrospective page via the atlas create operation. An existing page for the
   same week is preserved — offer to append to it instead of replacing it.
7. Offer to add each strong piece of evidence to the objective it belongs to,
   via the atlas append operation under that objective's evidence section. Confirm each
   one separately; nothing enters an objective without the owner agreeing to it.
   If a page declares the same evidence heading more than once, the operation
   refuses the write rather than guessing which objective was meant — name the
   objective's own heading instead.
8. If the week produced a durable claim about how this kind of work goes — not
   what happened, but what it suggests is generally true — offer to promote it
   to `owner/learnings/` via the atlas create operation, one page per claim. State it as a
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

## Autonomous mode (scheduled run)

Entered only when the invoking prompt states explicitly that this is an
unattended, scheduled run (`scheduled-tasks`) with no owner present. Never
inferred. If the prompt does not say so, the run is attended and the workflow
above applies as written. This mode relaxes no invariant except the two named
at the end of this section.

The standing-grant paragraph after step 9 already describes this shape: an
occurrence woken under the weekly retro grant writes the retrospective and not
a learning. This section says what the rest of the walk does under the same
grant.

1. **Steps 1 through 5 run without dialogue.** Read the week's dailies, the
   objectives and the previous retrospectives; name the patterns; land on one
   intention. Where step 3 would ask before concluding, state the reading and
   mark it: `evidência direta` when a daily page supports it, `inferido, não
   confirmado` when it does not. Never invent evidence to fill a quiet week —
   a thin week produces a short retrospective that says the evidence was thin.
2. **The page is written, with its intention marked as proposed.** Step 6 runs
   as usual, and an existing page for the same week is still preserved rather
   than replaced. The intention from step 5 is written as `proposta, a
   confirmar`, because no owner settled on it. One added line at the top of
   the page: `**Registro:** retro automática, não confirmada pelo dono`.
3. **Steps 7 and 8 do not run.** No evidence is added to an objective's own
   heading and no learning is promoted. Where the week surfaced something that
   would ordinarily prompt either, list it on the retrospective page under a
   `## Pendências desta rodada` heading — one line each, naming the objective
   or the candidate claim — so the owner can act on it in the next attended
   pass. This is the standing grant working as designed, not a limitation to
   route around: a durable claim about the owner's profession is theirs to
   make.
4. **A `proposed` result is final in this mode.** There is nobody to show the
   proposal to, so the page moved under the read and nothing is written: leave
   the owner's version alone, do not retry, and name it in the report.
5. **Report the retrospective in full to the chat of this execution.** The
   week's reading, the patterns, the proposed intention, which readings were
   inferred rather than evidenced, every pendência left behind, and whether
   the page was written, proposed or skipped. A retrospective the owner never
   hears about does not close their week.

### Invariants (autonomous mode)

- Autonomous mode is entered only on an explicit, self-declared unattended
  run. Never inferred from context.
- An autonomously written retrospective marks itself, its proposed intention
  and every uncertain reading as such, in its own text.
- No evidence is added to an objective and no learning is promoted in this
  mode, without exception. Both stay attended-only acts.
- Every other invariant of the attended skill still holds, with two named
  exceptions: the agreement gate is replaced by the markers in point 2, and a
  `proposed` result ends the write instead of starting a conversation.
- The interaction profile calibrates how much is explained, never how much of
  this run is reported. A concise profile shortens the prose, not the record:
  the report still carries every item, every marker and everything deferred.
## Invariants

- The skill never writes a file. Every effect is a named operation through the
  installed adapter.
- Nothing is recorded that the owner did not agree to in the conversation.
  The single exception is a scheduled run, which records a reading marked as
  unconfirmed with its intention marked as proposed — see "Autonomous mode"
  above.
- Evidence is quoted from what the owner wrote, never invented to fill an
  objective that had a quiet week.
- Client and engagement content stays in the workspace that owns it. A
  retrospective may name the engagement worked on; it does not copy findings,
  figures or deliverable material into owner scope.
- A write that reports `proposed` rather than `written` means the page changed
  underneath the read and nothing was persisted. Show the owner the proposal
  and let them decide; do not retry over their edit. In a scheduled run there
  is nobody to show it to — see "Autonomous mode" above.
- If an operation is unavailable, say so and continue the conversation. The
  walk, the reflection and a reviewable draft are all still worth having — only
  the recording is lost, and it must not be reported as done.
