---
name: qa-gate
description: Build an evidence-gated QA packet for a bounded change and classify it as pass, hold or unavailable without self-approving release. Use for "QA this change", "gate this release", "classify quality", or before shipping a deliverable that needs evidence.
---

> **Audience:** agent-facing only. This skill is not surfaced to the human owner.

# QA Gate

## Interaction profile

Resolve the canonical `interaction-profile` skill before presenting results to any agent that surfaces output to the owner. It adjusts explanation depth only; it never changes the QA verdict, evidence requirements, or scope.

Turn a consulting output into a proportionate quality decision. This skill
coordinates evidence across five dimensions and produces a verdict; it does not
replace the case owner's judgment or approve delivery.

## Workflow

1. Identify the output artifact, its stated purpose, the intended audience and
   the decision the evidence must support.
2. Read any case-level instructions and list the applicable quality checks
   before evaluating anything.
3. Evaluate the five QA dimensions below. For each, classify the signal as
   `pass`, `hold` or `unavailable`. A missing or inconclusive check is not a
   pass.
4. Produce a QA packet with an overall verdict of `PASS`, `HOLD` or
   `UNAVAILABLE`, the dimension-level findings, residual risk and the smallest
   next action.

## Five QA dimensions

### 1. Storyline soundness
Does the argument flow according to the Pyramid Principle? The governing
insight must appear first. Supporting points must be mutually exclusive,
collectively exhaustive and traceable to the governing claim. Flag any
inversion, redundancy or unsupported logical jump.

### 2. Evidence traceability
Every material claim must name a source. Check that each assertion is
accompanied by a reference — document, interview, dataset or public source —
that a reviewer could locate. Flag unsourced claims as `hold`.

### 3. Source credibility
Is the cited evidence credible for the stated claim? Consider recency,
authority, methodological fit and whether the source actually supports the
specific assertion made. A credible-looking citation that does not back the
claim is a traceability failure, not a pass.

### 4. Scope compliance
Does the output stay within what was explicitly asked? Flag scope creep,
missing deliverable sections and any recommendation that falls outside the
agreed brief. Scope decisions belong to the case owner, not the QA layer.

### 5. Tone and format fit
Is the output appropriate for the stated audience and channel? Check register
(executive vs. working-team), length, visual density and BCG editorial
conventions (conclusion-first, no bullet padding, no em-dash in external text).
Flag mismatches between stated audience and actual register.

## Output contract

The QA packet includes: output scope and version, dimension-level verdicts,
specific findings per dimension, residual risk and the one action required to
move a `HOLD` to `PASS`. Human review and delivery approval remain outside
this skill.

## Invariants

- Never convert a high score on one dimension into proof of overall fitness.
- Never hide a `hold` finding to produce a cleaner verdict.
- Never approve delivery — that decision belongs to the case owner.
- Never store client content, credentials or source bodies in the packet.
