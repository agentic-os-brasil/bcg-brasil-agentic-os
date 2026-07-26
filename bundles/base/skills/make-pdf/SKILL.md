---
name: make-pdf
description: Prepare user-supplied content for a future PDF by producing a print-ready structure and preflight checklist without rendering or saving a document. Use for “make this a PDF”, “prepare a shareable memo” or “is this ready to print?”.
---

# Make PDF

Resolve the canonical `interaction-profile` before responding. It changes
presentation only; it never grants renderer, filesystem or distribution authority.

## Contract

- Accept only content, audience and format constraints supplied in the current
  request.
- Return a print-ready outline, required sections, page/layout assumptions and
  a preflight checklist for source, citations and confidentiality.
- Identify missing source material or unsupported formatting honestly.
- Do not write Markdown, render PDF, save an artifact or claim it is ready for
  external distribution.

## Completion

Return a production specification. A deterministic renderer and governed
artifact/distribution path remain unavailable until separately implemented.
