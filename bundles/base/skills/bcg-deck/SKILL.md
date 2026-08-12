---
name: bcg-deck
description: Build one decision-led professional storyline and deck plan from approved case evidence. Use inside an authorized workspace or account context; it does not authorize reading, editing or presentation delivery.
---

# Deck and Storyline

Use when a Case Agent needs to turn an approved decision, audience definition,
evidence set and constraints into a Pyramid-structured storyline and a
slide-by-slide deck plan. This is a planning method, not a deck-building or
presentation capability.

## Interaction profile

Resolve the canonical `interaction-profile` before presenting the plan. It
changes explanation depth only; it never changes evidence, source or the
planning output.

Ask one question at a time. If the user answers "não sei" or "not sure" for any
field, offer a sensible default and proceed. Never present more than one open
question in the same turn.

## Required input

- the decision or action the audience must be able to take after seeing the deck;
- audience type (e.g., partner alignment, client C-suite, internal steering);
- available evidence or analyses with their sources;
- constraints: target slide count, presentation time and communication tenor
  (persuade, inform, align, escalate).

If the decision, audience type or evidence list is missing, return
`unavailable/input_scope` with the missing fields and request them one at a
time. Never infer a client, decision or audience from a file path, project name
or prompt fragment.

## Method

1. State the governing decision and the definition of "audience aligned" for
   this specific context.
2. Separate confirmed facts, approved analyses, assumptions and open questions.
3. Build a Pyramid-structured argument: governing thought → key lines → supporting
   evidence for each line.
4. Map each key line to the smallest set of exhibits that prove or qualify the
   argument, with explicit source pointers from the supplied evidence.
5. Identify gaps: claims that cannot be fully supported by the supplied evidence,
   assumptions requiring validation and decisions that still need human sign-off.
6. Size the plan to the stated constraints (slide count, time, tenor) and flag
   any irreconcilable tension between the argument depth and the constraints.

## Output contract

Return a structured packet containing:

- `governing_thought`: the single sentence the audience must take away;
- `key_lines[]`: each with `claim`, `evidence_pointers[]` and `exhibit_type`;
- `slide_plan[]`: each slide with `number`, `action_title`, `claim`, `evidence`
  and `open_validations`;
- `assumptions[]`: each with `statement` and `validation_needed`;
- `gaps[]`: claims or exhibits that cannot be supported by the supplied evidence;
- `constraints_assessment`: whether the argument fits the stated limits, and any
  trade-offs required;
- `unavailable_checks`: anything that cannot be assessed with the supplied input.

## Invariants

- Does not invent client facts, numbers or sourced claims.
- Does not browse for additional context, data or external sources.
- Does not open, edit or generate an actual deck file (PPTX, PDF, DOCX or
  similar).
- Does not address presentation delivery, rehearsal or speaker notes.
- A gap in the evidence remains stated as a gap; it is never filled with
  plausible reasoning or generic BCG benchmarks.
- If the supplied evidence is insufficient to build a coherent storyline, return
  `unavailable/insufficient_evidence` and specify what is missing.
