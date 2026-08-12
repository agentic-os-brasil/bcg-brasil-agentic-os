---
name: bcg-case-kickoff
description: Build a bounded, decision-led kickoff plan for a new professional case from approved scope and workspace context. Produces structure only; it does not create files, schedule meetings or contact people.
---

# Case Kickoff

Use when a Case Agent needs to turn an approved case scope into a practical
first-days plan. This is a method, not a project system and not a grant to read
unapproved material.

## Interaction profile

Resolve the canonical `interaction-profile` before presenting the plan. It
changes explanation depth only; it never changes scope, evidence, authority or
the confirmation gates.

## Required input

- case or workspace identifier and owning Case Agent;
- approved scope or proposal pointer;
- start window, audience and known constraints;
- authorized evidence pointers, if any.

If scope, ownership or source authorization is missing, return
`unavailable/input_scope` with the missing fields. Never infer a client, team,
deadline or authority from a path or prompt fragment.

## Method

1. State the case objective, decision horizon and definition of done.
2. Separate confirmed scope, assumptions, open questions and explicit
   exclusions.
3. Build a phased plan: pre-start alignment, day-one setup and first
   hypothesis/evidence cycle.
4. For each phase, define outcome, owner role, approved inputs, dependency and
   smallest useful next action.
5. Add a meeting-cadence proposal only as a draft for human confirmation; do
   not schedule or send anything.
6. Identify risks, missing access and decisions that require Walter review.

## Output contract

Return a structured packet with `objective`, `scope_boundary`, `phases`,
`open_questions`, `assumptions`, `dependencies`, `review_required` and
`unavailable_checks`. Each phase contains `outcome`, `actions`, `owner_role`,
`approved_sources` and `next_safe_action`.

## Invariants

- Do not create DOCX, slides, tasks, calendar events or messages.
- Do not browse, ingest or open a source unless an authorized capability and
  explicit source pointer are supplied by the owning agent.
- Do not claim that a Case Agent, PA Expert, Walter or
  Darwin was invoked; this skill only produces a method packet.
- A missing required capability remains `unavailable`; do not emulate it with
  an external provider or hidden persistence.
