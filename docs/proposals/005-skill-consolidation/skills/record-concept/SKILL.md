---
name: record-concept
description: Records a reusable method, framework, or playbook into concepts — an established technical way of doing something (e.g., the order and framework for a spend-cube analysis) worth reusing across projects. Use when a repeatable approach has proven itself and should be captured so it isn't reinvented next time.
---

# Record Concept

A concept is reusable technical craft — the "how we always do X" — distinct from a career
learning (`record-learning`) or a one-off case decision (`decision-log`). Executed by
`career-keeper`.

## Method
1. Confirm the method is genuinely reusable and has proven itself, ideally across more than
   one project — not a one-off.
2. Add a concept file under `concepts/` (or extend an existing one): what it is, when to
   use it, the steps/framework, and a pointer to any runnable tooling or example.
3. Add one line to the concepts index so it's discoverable, and link related concepts.

## Relations
- **Executed by `career-keeper`** (owner of `concepts/`).
- Invocable by the **hub**, or surfaced by an analyst (`quant-analyst` / `quali-analyst`)
  that discovers a method worth keeping — they surface it, `career-keeper` files it.
