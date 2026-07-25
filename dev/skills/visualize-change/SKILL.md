---
name: visualize-change
description: Document a material repository change with evidence-backed Mermaid diagrams and concise navigation. Use when an implementation spans several contracts or components, when a user asks for architecture or flow visualizations, or when READMEs, specs and dedicated visual documentation must stay aligned with what is implemented, validated, runtime-active and still planned.
---

# Visualize Change

Turn the real repository state into a small set of durable views that help both
humans and agents understand the change. This complements `$develop-change`; it
does not replace implementation evidence or validation.

## Workflow

1. Read the actual diff, relevant specs, accepted decisions and validation
   evidence. Do not infer current behavior from roadmap language alone.
2. Separate four states explicitly: implemented, validated, runtime-active and
   planned. Never render a definition or catalog entry as an enabled runtime.
3. Choose the smallest useful Mermaid view:
   - `flowchart` for topology, boundaries, ownership or allowed routes;
   - `sequenceDiagram` for delegation, review or data exchange;
   - `stateDiagram-v2` for activation gates or lifecycle state.
4. Update the canonical spec first. Keep the root README to orientation and
   links; create `docs/visualizations/<topic>.md` when several views or detailed
   operational notes are needed.
5. Add one or two prose sentences around every diagram so the meaning survives
   a renderer failure and the present-state caveat is unambiguous.
6. Link the visualization from the nearest navigation surface. Avoid copying
   the same large diagram into several files.
7. Run `go run ./dev/harness validate` while iterating and
   `go run ./dev/harness validate --full` before delivery. The harness checks
   Mermaid fence integrity and supported diagram headers offline.
8. Report which documentation changed, which repository evidence it reflects
   and which runtime or product gaps remain.

## Mermaid rules

- Use stable, short node IDs and quote labels containing punctuation.
- Keep direction consistent within a view and label only load-bearing edges.
- Prefer several focused views over one dense graph.
- Never embed credentials, client names, personal data, raw workspace paths or
  operational memory in a diagram.
- Do not use color alone to encode status. State the status in the node label or
  adjacent prose.
- Keep diagrams runtime-neutral unless the document is explicitly an adapter
  surface.

If a diagram would merely restate a short paragraph or table, keep the prose or
table. Visuals are for relationships, sequences, boundaries and state changes.
