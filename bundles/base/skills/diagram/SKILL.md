---
name: diagram
description: Turn a user-supplied process, hierarchy, issue tree, sequence or timeline into an editable diagram specification without rendering or saving a file. Use for “diagram this”, “draw the process”, “make a flowchart” or “show this as a 2x2”.
---

# Diagram

Resolve the canonical `interaction-profile` before responding. It changes
presentation only; it never grants file, renderer or external-tool authority.

## Contract

- Accept only the description and labels supplied in the current request.
- Return a selected diagram type, nodes, edges, layout direction and editable
text specification such as Mermaid when appropriate.
- Flag ambiguous ownership, sequence or measures before rendering.
- Do not read source files, invoke a renderer, save SVG/source or claim an
artifact exists.

## Completion

Return an advisory specification. Deterministic rendering and artifact storage
remain unavailable until their separate contract exists.
