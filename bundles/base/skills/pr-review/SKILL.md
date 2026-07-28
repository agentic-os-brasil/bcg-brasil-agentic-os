---
name: pr-review
description: Review a pull request with a compact evidence pack, deterministic gates, risk-based depth and a human-readable verdict without merging it.
---

# Pull-Request Review

Review the current revision, not a remembered branch state. The skill produces
a verdict and suggested feedback; merge, deployment and approval remain
separate human or adapter actions.

## Interaction profile

Resolve the canonical `interaction-profile` before presenting review findings.
The profile changes presentation depth only; the review gates and verdict rules
remain identical.

## Stage 0 — deterministic gates

Collect or reuse a metadata-only `PR_EVIDENCE_PACK` containing repository and
PR identity, base/head revisions, changed-file summary, draft state,
mergeability, human reviews, required-check rollup, automated-review signals
when available, risk triggers and focused hunks. Before any deep reading:

- hold a draft unless the request explicitly asks for draft feedback;
- flag stale, dirty, conflicting or unknown mergeability;
- check that required checks refer to the current head revision;
- record missing human review or required evidence;
- note whether the change touches contracts, security, data boundaries,
  migrations, public interfaces or release behavior.

Do not infer a repository-specific gate. If a provider or check is absent,
record `unavailable` rather than inventing a pass.

## Risk-based depth

- **quick:** small diff, current revision, no material risk trigger and fresh
  green evidence;
- **standard:** normal feature or refactor, changed behavior and focused
  contract/test review;
- **deep:** conflict, stale evidence, security/data/migration risk, failed or
  contradictory signals, cross-branch dependency or material public API change.

Never use quick depth to skip Stage 0. Full-diff review is an escalation, not a
default.

## Review dimensions

Check correctness and invariants, contract/API compatibility, tests and
failure paths, security/data boundaries, observability/rollback and
documentation. Treat automated reviewer comments as signals to verify, not as
the verdict. Separate confirmed findings, false positives and open questions.

## Verdict

Return one of:

- `APPROVE` — evidence is current and no material blocker remains;
- `REFINE` — one to three concrete changes would resolve the review;
- `HOLD` — deterministic gate, conflict, missing required evidence or material
  unresolved risk blocks a decision;
- `UNAVAILABLE` — the requested review cannot be qualified in this runtime.

Include the top material findings, evidence pointers, explicit limitations and
the next reversible action. Do not reproduce full diffs, logs or sensitive
payloads.

## Anti-patterns

- reviewing before the evidence pack and Stage 0;
- copying an automated score as approval;
- repeating every bot comment instead of synthesizing material findings;
- approving a stale head or treating skipped checks as green;
- merging, pushing or editing another contributor's branch in this skill.
