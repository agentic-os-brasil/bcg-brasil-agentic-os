---
name: investigate
description: Diagnose a surprising result or broken outcome through explicit symptoms, hypotheses and verification steps before proposing a fix. Use for “investigate this”, “why is this wrong?”, “this does not add up” or “debug this”.
---

# Investigate

Resolve the canonical `interaction-profile` before responding. It changes
explanation depth only; it never grants file, data, tool or edit authority.

## Contract

- Accept a user-supplied symptom, expected-versus-actual behavior and optional
  evidence references.
- Return a precise symptom, ranked hypotheses, evidence required for each,
  one verification step and a stop/reframe condition.
- Distinguish observation, hypothesis and conclusion.
- Do not inspect files, run commands, modify an artifact or present a cause as
  established without supplied evidence.

## Method

1. Define the observed and expected outcomes precisely.
2. Trace the smallest causal chain visible in the supplied evidence.
3. Rank falsifiable hypotheses; test one before moving to the next.
4. After three failed hypotheses, reframe rather than add a fourth guess.

## Completion

Return an investigation plan, not a fix. A later authorized execution may
collect evidence and apply a verified correction with its own receipt.
