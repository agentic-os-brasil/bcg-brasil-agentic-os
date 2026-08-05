---
name: coverage-diagnose
description: Diagnose test coverage and rank the next highest-value test targets without writing tests or treating a percentage as proof of quality.
---

# Coverage Diagnose

Produce a compact, reproducible coverage map for a bounded source area. This is
an atomic diagnostic: it does not write tests, change a coverage floor, or
approve a release.

## Interaction profile

Resolve the canonical `interaction-profile` before presenting the map. The
profile changes explanation depth only; it never changes evidence, permissions
or data scope.

## Workflow

1. Read the repository's local instructions, test configuration and ownership
   rules. Do not infer ownership from a path name.
2. Identify the configured coverage command. If execution is authorized,
   run the smallest command that produces per-file or per-module evidence;
   otherwise report the command that would be needed.
3. Record only metadata: command, tool/version when available, baseline,
   covered and missed targets, and whether the run passed.
4. Rank candidates by blast radius, public boundary, complexity/churn and
   uncovered behavior. A low percentage is a signal, not the ranking by
   itself.
5. Recommend one bounded next test wave with an explicit reason and a
   negative or failure path to cover.

## Output

Return:

- baseline and command evidence;
- top targets with risk rationale;
- ownership or access gaps as `unknown`, never guessed;
- one recommended next wave;
- unavailable checks and residual risk.

## Invariants

- Coverage does not prove business fitness, correctness or release readiness.
- Do not copy source, client data, prompts, credentials or full test output
  into the report.
- Do not rewrite tests, source files or configuration as a side effect.
- A missing tool, fixture or permission is an explicit gap, not a pass.
