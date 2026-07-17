---
name: develop-change
description: Implement features, fixes and refactors in the BCG Brasil Agentic OS source repository through its development-only harness. Use for code or behavioral changes that require classification, relevant specs or project decisions, contract-focused tests, minimal implementation and fast/full validation; also use to identify when documentation, mechanical changes or spikes do not justify artificial decisions or tests.
---

# Develop Change

Keep development evidence recoverable while matching the ceremony to the risk of the change.

## Workflow

1. Read `AGENTS.md`, `docs/decisions/decision-log.md` and the relevant spec.
2. Classify the change using `specs/005-development-harness.md`.
3. Run `go run ./dev/harness validate` to establish a clean fast-loop baseline.
4. For a durable product, architecture, security, data, runtime or development decision, use `$record-decision` before implementation.
5. For behavioral work, write the smallest test that expresses the observable contract and confirm it fails for the expected reason.
6. Implement the smallest coherent change that satisfies the test. Avoid coupling tests to private implementation details.
7. Re-run the targeted test and fast harness during iteration.
8. Run `go run ./dev/harness validate --full` before closing.
9. Report separately what was implemented, what test-first evidence was observed, what validations passed and what remains unvalidated.

If an agent sandbox blocks the default Go build cache, point `GOCACHE` to a runtime temporary directory and rerun the same command. Do not create cache state inside the repository.

## Classification

- Behavioral contract or architecture: require a spec or durable decision and contract evidence.
- Bug: reproduce with a failing test before fixing.
- Risk-bearing refactor: add characterization tests when current behavior lacks protection.
- Documentation, copy or mechanical change: run applicable validation without inventing a decision or meaningless test.
- Spike: time-box it and label it non-production; return to the normal flow before incorporation.

Never claim test-first when the test was written after implementation. Never weaken a test merely to make the harness green.

This skill and the harness are development tools only. Do not add them to the user CLI, OS bundle or pilot installation.
