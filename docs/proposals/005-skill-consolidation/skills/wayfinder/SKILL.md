---
name: wayfinder
description: Turns a fuzzy, tangled problem into a structured MECE issue-tree or hypothesis-tree, so it can be resolved one branch at a time. Use for "help me structure this", "I'm stuck / where do I start", "map this out", "break this down", or as the framing step before planning or analysis.
---

# Wayfinder

The proposed problem-structuring method. It produces a bounded tree; it does not resolve
the branches (planning is `work-planner`, casework analysis is `quali-analyst`). This
proposal-level file is not a managed runtime skill until its promotion contract and
evaluations pass.

## Contract

- **Input:** one natural-language problem statement plus optional owner-supplied
  constraints. Do not fetch context, infer a client, or search for missing facts.
- **Output:** a framed question, 3–6 MECE branches, branch priorities, assumptions, and
  one recommended first branch. Output is advisory and must carry a correlation ID.
- **Denied authority:** no filesystem, memory, task, calendar, network, connector,
  delegation, or external disclosure. It must not create or update a durable artifact.
- **Failure behavior:** if the problem is underspecified, state the missing constraint and
  ask one bounded clarification; never invent evidence or claim a branch was resolved.
- **Runtime parity:** Claude and Codex may phrase the tree differently, but must preserve
  the same fields, branch count bound, and denial behavior.

## Method
1. **Frame the real question** in one line — reframe if the stated question isn't the
   ambiguity that matters.
2. **Decompose** into 3-6 mutually exclusive, collectively exhaustive sub-questions — a
   real tree, not a flat list.
3. **Prioritize** the branches by impact × ease (or by what most changes the answer).
4. Return the tree plus the single highest-value branch to attack first.

## Relations
- **Shared method, proposed for three actors** — this is why it's a skill and not buried in an
  agent body:
  - the **hub**, for the user's own personal problem/task structuring;
  - **`quali-analyst`**, applied to a case problem that becomes part of a deliverable;
  - **`work-planner`** (TASK mode), as the framing step before it sequences the work.
- Each of those pulls this skill rather than re-describing MECE structuring.
