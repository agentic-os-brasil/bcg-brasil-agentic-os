---
name: expert-interview-guide
description: Structure a professional expert interview guide from an approved case question, audience and source set; returns questions and preparation logic without browsing or creating documents.
---

# Expert Interview Guide

Use for a bounded one-to-one interview with a customer, competitor, industry
expert, channel partner, supplier or internal stakeholder. It produces a guide,
not a contact action or a research result.

## Interaction profile

Resolve the canonical `interaction-profile` before presenting the guide. It
changes explanation depth only; it never changes the approved scope or what
may be disclosed externally.

## Required input

- workspace and case scope;
- interviewee type and role, without inventing identity;
- decision or hypothesis the interview must inform;
- authorized evidence pointers and known constraints;
- duration, language and any approved topics.

If the decision, scope or sources are missing, return
`unavailable/input_scope`. Do not research the interviewee or infer sensitive
facts from a name.

## Method

1. Translate the decision into 3–6 distinct learning objectives.
2. Build a sequenced guide: opening, context, core questions, probes,
   quantification prompts, contradictions and close.
3. Tag questions `[MUST-HAVE]`, `[IMPORTANT]` or `[OPTIONAL]`; tag every
   metric question `[QUANTIFY]`.
4. Separate externally safe questions from internal interviewer notes. Internal
   notes may contain only supplied, authorized evidence pointers and hypotheses.
5. Add neutrality, consent, confidentiality and time-boxing reminders.
6. Mark assumptions, missing evidence and questions requiring owner approval.

## Output contract

Return `learning_objectives`, `external_guide`, `internal_prep_notes`,
`question_priority_counts`, `assumptions`, `approved_sources`,
`disclosure_checks`, `review_required` and `unavailable_checks`.

## Invariants

- No browsing, transcript search, contact lookup, outreach, document creation
  or persistence.
- Never expose internal hypotheses, client context or unapproved evidence in
  `external_guide`.
- Do not invent a PA Expert or source consultation.
- A required research or document capability remains `unavailable` until its
  adapter is qualified; the guide may still be returned as a method packet.
