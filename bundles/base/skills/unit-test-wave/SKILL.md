---
name: unit-test-wave
description: Orchestrate a bounded test wave from coverage diagnosis through focused validation, strict bug capture and metadata-only evidence.
---

# Unit-Test Wave

Run a repeatable, surgical wave against one module, boundary or behavior. The
wave is an engineering procedure, not permission to edit another owner's code,
merge a branch or lower a quality bar.

## Interaction profile

Resolve the canonical `interaction-profile` before presenting the wave plan.
Use plain language for standard users and expose commands and diagnostics only
when requested; the quality bar and data boundaries remain unchanged.

## Sequence

1. Confirm scope, repository instructions, ownership and a clean enough
   working state. Preserve unrelated changes.
2. Invoke `coverage-diagnose` or collect a baseline with the repository's
   configured command.
3. Prioritize public boundaries, orchestrators and high-blast-radius logic;
   then lower-level helpers. State why the target was selected.
4. Read the target module, its tests and local conventions before writing.
5. Choose a test strategy: pure unit tests first; deterministic fakes for
   dependencies; integration or end-to-end only when the risk requires it.
6. Define a matrix before implementation: happy path, edge cases, invalid
   input, dependency failure, invariants, output contract and constants or
   configuration that can drift.
7. Add tests in the repository's established location and style. Keep helpers
   local, names behavior-focused and fixtures minimal.
8. Run focused tests iteratively. Classify failures as test drift, environment
   gap, contract change or production bug.
9. Invoke `xfail-bug-capture` only for a confirmed production bug; strict
   expected failures remain linked to a durable issue or decision record.
10. Re-run focused tests, then the smallest broader suite that protects the
    changed boundary. Do not hide skipped or unavailable checks.
11. Raise a coverage floor only when the repository's policy supports it and
    the new baseline is justified by the wave; never edit a floor to obtain a
    green result.
12. Lint and format only files owned by the wave. Do not turn pre-existing
    global lint debt into unrelated edits.
13. Return the evidence packet. A human or authorized adapter performs any
    commit, push, review or merge as a separate action.

## Output contract

Report baseline → result, tests added, commands and outcomes, bugs or strict
expected failures, files changed, unavailable checks, residual risk and the
next bounded wave. Use pointers and summaries; do not persist source bodies,
client data, credentials or full logs.

## Quality triplet

Judge the wave with three signals: behavior covered, regressions prevented and
real bugs found. Coverage percentage alone is insufficient.

## Anti-patterns

- writing tests in another ownership boundary without a handoff;
- global lint or full-repository rewrites for a focused wave;
- treating a skipped test or missing fixture as success;
- using a non-strict expected failure to suppress a regression;
- bundling the wave with merge or release authorization.
