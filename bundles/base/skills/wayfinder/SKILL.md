---
name: wayfinder
description: Structure a fuzzy professional question into a bounded MECE issue tree and identify the first branch to investigate. Use for “help me structure this”, “where do I start?”, “map this out” or before planning and analysis.
---

# Wayfinder

Resolve the canonical `interaction-profile` before responding. It changes
explanation depth only; it never grants data, tool or delegation authority.

## Contract

- Accept one user-supplied question and optional constraints.
- Return a framed question, 3–6 distinct branches, stated assumptions,
  branch priorities and one first branch to investigate.
- Ask one bounded clarification when essential context is missing.
- Do not retrieve context, read files, delegate, create a task, mutate state or
  claim that a hypothesis is true.

## Method

1. State the decision or uncertainty that matters.
2. Divide it into mutually distinct branches that together answer the question.
3. Rank branches by decision impact and ease of validation.
4. Name the smallest useful next investigation.

## Completion

Return an advisory tree only. If later work needs evidence, artifacts or a
durable execution, route it through the separately authorized workspace flow.
