---
name: decision-log-entry
description: Turn supplied decision language into a reviewable, source-grounded project decision record draft without writing the project log.
---

# Decision Log Entry

Use when a Case Agent has a decision that may need a durable project record.
The skill drafts the entry; the owning workflow decides whether and where to
persist it.

## Interaction profile

Resolve the canonical `interaction-profile` before presenting the draft. It
changes explanation depth only; it never changes the record, approval or
persistence rules.

## Required input

- verbatim or clearly marked decision statement;
- workspace/project scope and decision date;
- decision maker or group when explicit;
- linked evidence/spec pointers, if any;
- review horizon when required by the project contract.

If the decision, owner or project scope is missing, return
`unavailable/input_scope`. Never infer a decision maker, identifier, sequence
number or linked source.

## Method

1. Preserve quoted language and distinguish it from synthesis.
2. Classify the record as methodology, architecture, scope, stakeholder,
   approval, amendment or pending only when supported by the input.
3. Draft context, decision, rationale, consequences, linked evidence, open
   questions and a proposed review date.
4. Mark unresolved fields as `pending` and list the confirmation needed before
   persistence.
5. Return a stable record body plus a machine-readable field map; do not append
   to a log.

## Output contract

Return `record_draft`, `fields`, `verbatim_quotes`, `linked_sources`,
`confirmation_required`, `review_target`, `persistence_status: not_attempted`
and `unavailable_checks`.

## Invariants

- No file write, memory commit, issue creation, notification or external
  publication.
- No automatic `D-NNN` or other project identifier assignment.
- Do not turn an open question, preference or hypothesis into a locked
  decision.
- Walter review may be required by the owning governance workflow; the skill
  must not claim that Walter approved the draft.
