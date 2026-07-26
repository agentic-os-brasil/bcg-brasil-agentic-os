---
name: diagram
description: Turns an English description into a diagram — process flow, org chart, issue-tree, 2x2, sequence, or timeline — rendered to SVG plus editable source. Use for "make a diagram/flowchart of...", "draw this", "diagram the process". For a polished client-deck exhibit, use deck-builder instead.
---

# Diagram

A thinking/draft diagram for memos and internal alignment, not a client-deck exhibit.
Low freedom: the render step is a fixed script.

## Method
1. Pick the diagram type that fits the intent (flow, tree, 2x2, sequence, timeline).
2. Write clean diagram source — tight labels, left-to-right where it reads better.
3. Render it with the project's diagram renderer (exact command, no extra flags).
4. Return the rendered SVG path + the editable source, and note it can be re-edited in a
   diagram editor.

## Relations
- **Tool-wrapper, multi-consumer** — used by the **hub** or **any agent** that needs a
  quick structural picture (e.g., `deck-builder` for a draft exhibit before the polished
  version). Rendering is local, so confidential content is safe.
