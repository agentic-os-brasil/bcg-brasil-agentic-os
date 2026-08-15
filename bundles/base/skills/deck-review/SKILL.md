---
name: deck-review
description: Review supplied slide text for storyline, consistency, action titles, evidence limits and language issues without opening, editing or generating a deck. Use for "review my deck", "pressure-test these slides", "check the storyline", or before a review with a senior stakeholder.
---

# Deck Review

Use when a Case Agent supplies slide text or an approved text extraction and
needs a structured quality review. This is a review method, not a PowerPoint or
spreadsheet capability.

## Interaction profile

Resolve the canonical `interaction-profile` before presenting findings. It
changes explanation depth only; severity, evidence and release gates do not
change.

## Required input

- complete slide text with stable slide boundaries;
- decision, audience and approved scope;
- optional transcript or review requests, each explicitly in scope.

If the deck is supplied only as a local file and no qualified extraction
capability is available, return `unavailable/deck_extraction` and stop. Do not
open a file, browse or claim a visual review.

## Method

1. Read the executive summary and identify the central argument.
2. Track repeated numbers, claims and definitions across all supplied slides.
3. Review five dimensions: storyline, action titles, consistency, evidence /
   footnotes and language.
4. Classify findings as `critical` when they can mislead or block delivery, or
   `advisory` when they improve clarity without invalidating the argument.
5. Reconcile supplied transcript requests as `addressed`, `not_addressed` or
   `unclear`.
6. Return a verdict with unresolved risks and the smallest fix sequence.

## Output contract

Return `verdict`, `top_issues`, `findings[]` with `slide_refs`, `severity`,
`dimension`, `evidence` and `suggested_fix`, `transcript_checks`,
`validation_limits` and `unavailable_checks`.

## Invariants

- Never edit or generate PPTX, PDF, DOCX, XLSX or images.
- Never invent a number, visual property, source or transcript outcome.
- A text-only review cannot certify layout, rendering, footnotes hidden in
  visuals or accessibility; state those limits explicitly.
- Yoda review remains a separate governance step when the verdict has
  material decision or delivery consequences.
