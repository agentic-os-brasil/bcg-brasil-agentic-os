---
name: make-pdf
description: Turns a Markdown file (memo, one-pager, self-review pack) into a publication-quality, self-contained document ready to print or share. Use for "make a PDF/document of this", "turn this memo into a shareable doc". Not for slide decks — use deck-builder for those.
---

# Make PDF

Markdown in, a polished self-contained document out. Low freedom: the conversion is a
fixed script.

## Method
1. Get or write the Markdown source.
2. Run the project's document renderer on it (exact command).
3. Return the output path; if a print-to-PDF step is needed, give the one instruction.

## Relations
- **Tool-wrapper, multi-consumer** — used by the **hub** or **any agent** producing a
  human-readable document (e.g., `career-keeper` for a self-review pack before a career
  conversation). Rendering is local.
