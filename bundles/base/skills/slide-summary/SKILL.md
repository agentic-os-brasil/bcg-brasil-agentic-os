---
name: slide-summary
description: Convert supplied deck text into a slide-by-slide message map and narrative arc without extracting files, saving content or creating a deck. Use for "summarize this deck", "map the slide messages", "what's the storyline", or when only the arc of an existing deck is needed.
---

# Slide Summary

Use when an approved text extraction already has slide boundaries and needs a
structured summary for a Case Agent. The slide title is treated as the
candidate message, not merely a section label.

## Interaction profile

Resolve the canonical `interaction-profile` before presenting the map. It
changes explanation depth only; it never changes source scope or inference
rules.

## Required input

`slide_text` with stable boundaries is required. `deck_title`, `workspace_id`,
`max_slides` and an explicit delimiter are optional. If the text is absent,
ambiguous or exceeds the declared cap, return `unavailable/input_scope` or a
bounded partial result with a warning. Never open a local deck without a
qualified extraction capability.

## Method

1. Split the supplied text into slides using the declared delimiter or a
   conservative, documented heuristic.
2. Extract `title`, `message`, 2–5 `key_points` and `type` for each slide.
3. Capture `so_what` only when the body states an implication; otherwise use
   `null`.
4. Compose a short `deck_arc` describing the movement from opening to
   conclusion, preserving uncertainty.
5. List unused or ambiguous slides separately; do not silently discard them.

## Output contract

Return JSON-serializable data with `slides`, `total_slides`, `deck_arc`,
`unused_slides`, `warnings` and `unavailable_checks`. Each slide contains
`slide_num`, `title`, `message`, `so_what`, `key_points` and `type`, where type
is one of `agenda`, `content`, `recap`, `next_steps`, `unused` or `unknown`.

## Invariants

- Never infer a strategic implication without textual evidence.
- Never call an extractor, browser, SharePoint, Notion or another agent.
- Never persist slide text, client names or summaries in the managed bundle,
  telemetry or memory.
- If extraction is required but unavailable, fail closed on the extraction
  step while preserving only the explicit limitation.
