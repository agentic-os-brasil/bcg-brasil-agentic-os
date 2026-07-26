---
name: test-and-evidence
description: Define proportionate verification for a software, data or technical artifact and separates evidence from assertion. Use before reviewing a change, analysis pipeline or reusable technical deliverable.
---

# Test and Evidence

Build confidence through observable checks matched to risk. This is a
professional delivery workflow, not permission to run unreviewed code, access
production data or self-approve a release.

## Interaction profile

Resolve the canonical `interaction-profile` before explaining validation. Use
one recommended route for standard users and expose additional diagnostics only
for advanced or power users. The profile does not relax a test, data or approval
standard.

## Workflow

1. Identify the artifact, intended user, decision it supports and material
   failure modes.
2. Choose the smallest appropriate evidence: unit test, data contract check,
   comparison against a baseline, peer review, reproducible rerun or manual
   acceptance scenario.
3. Define expected success and expected failure behavior before changing the
   artifact when behavior is being introduced or repaired.
4. Execute only approved checks in an authorized environment. Record commands,
   versions and result summaries without copying sensitive payloads.
5. Distinguish what passed, what was not tested and what remains an assumption.
6. Escalate a failed check, missing fixture or insufficient access rather than
   weakening criteria to produce a green result.

## Invariants

- Never represent a test written after a change as test-first evidence.
- Never convert a coverage percentage into proof of business fitness.
- Never persist credentials, raw client data, prompts, tool payloads or error
  bodies in the evidence summary.
- Human review and release authorization remain separate from test execution.
