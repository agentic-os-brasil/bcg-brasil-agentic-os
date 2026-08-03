---
name: qa-gate
description: Build an evidence-gated QA packet for a bounded change and classify it as pass, hold or unavailable without self-approving release.
---

# QA Gate

Turn a change request into a proportionate quality decision. This skill
coordinates checks and evidence; it does not grant production access, approve
a release or replace human review.

## Interaction profile

Resolve the canonical `interaction-profile` before presenting the QA packet.
The profile controls communication depth only and cannot relax a required
check or approval boundary.

## Workflow

1. Identify the changed artifact, intended behavior, risk surface and the
   decision the evidence must support.
2. Read repository-local instructions and list required checks before running
   anything. Prefer configured commands and existing fixtures.
3. Select the smallest sufficient set across build/type checks, unit tests,
   integration or end-to-end tests, static analysis, contract checks and
   manual acceptance. Explain omissions.
4. Execute only authorized local checks, or record them as pending when an
   adapter or environment is required. Capture command, scope, result and
   revision metadata—not payloads.
5. Review test quality: meaningful assertions, failure paths, deterministic
   fixtures, isolation and regression coverage. Use `coverage-diagnose` or
   `unit-test-wave` when the change exposes a gap.
6. Classify each signal as `pass`, `fail`, `skipped`, `blocked` or
   `unavailable`. A skipped or unavailable required check is not a pass.
   For code changes, Maestro may request a bounded `code_quality` evaluation
   from Gamma Guardian. Keep its five-dimension result separate from command
   checks: a local signal is advisory, and native qualification remains
   `unavailable` without independent runtime evidence.
7. Produce a QA packet with verdict `PASS`, `HOLD` or `UNAVAILABLE`, residual
   risk and the smallest next action.

## Output contract

The packet includes scope/revision, required checks, executed checks, failures,
skips, evidence pointers, test-quality observations, residual risk and human
decision still required.

## Invariants

- Never convert a coverage number, automated score or green subset into proof
  of business fitness.
- Never rerun an expensive check merely because another skill already recorded
  fresh evidence for the same revision.
- Never hide a failed, flaky or environment-blocked check.
- Never store credentials, source records, prompts or full tool output.
