<!--
Use this template to make the PR understandable without reconstructing the
diff. Delete guidance comments before requesting review. State facts only:
never mark a check or capability that did not actually run or exist.
-->

# What this PR delivers

## 1. Executive summary

<!-- Two or three sentences in plain language:
     - What problem or opportunity existed?
     - What is different after this PR?
     - Who benefits or what risk is reduced? -->

## 2. What changed

<!-- Group changes by outcome, not by every file touched. -->

### Delivered

- **[Capability or outcome]** — what now works and how someone uses it.
- **[Guardrail or correction]** — what unsafe, confusing or incorrect behavior is prevented.

### Important implementation choices

<!-- Only decisions a reviewer needs to understand the solution. Include a
     spec or four-letter decision reference when applicable. -->

- **[Choice]** — why this approach was chosen.

## 3. Relevant impact

<!-- Fill only the surfaces affected by this PR. A single “No relevant impact
     beyond internal documentation” line is valid for simple changes. -->

| Affected audience or surface | Before | After |
| --- | --- | --- |
| [surface affected] |  |  |

## 4. How to review

<!-- Give the reviewer the shortest useful path through the diff. -->

1. Start with `[file or spec](path)` to understand the contract.
2. Review `[file or module](path)` for the implementation.
3. Confirm `[test or command](path)` covers the observable behavior.

## 5. Evidence

### Tests and checks run

<!-- List only commands actually run and their result. CI is populated after
     the PR is opened. -->

| Evidence | Result |
| --- | --- |
| `go run ./dev/harness validate --full` | pass / not run — reason |
| Targeted test: `…` | pass / not applicable — reason |
| CI: Windows, macOS, Linux | pending / pass / fail |
| Manual check: `…` | pass / not applicable — reason |

### Coverage of relevant behavior

<!-- Name the test, characterization or manual validation that protects the
     relevant behavior. For docs/mechanical work: state “not applicable —
     reason”. -->

## 6. Contracts, safety and data boundaries

### Decisions and specs

- Decision: `NONE` / `ABCD` — one-line consequence.
- Spec: `NONE` / `specs/NNN-...` — what changed or why none changed.

### Privacy and storage

- [ ] No client data, personal data, credentials, conversations or real memory is included.
- [ ] Managed core remains separate from local owner data and workspaces.
- [ ] New local writes are non-destructive, scoped and recoverable.
- Not applicable / exception and rationale:

### Runtime portability

- Claude: native / emulated / degraded / unavailable — explanation.
- Codex: native / emulated / degraded / unavailable — explanation.
- Shared contract or intentional difference:

## 7. What this PR explicitly does not do

<!-- Name nearby expectations that remain pending. This prevents a reviewer
     or pilot user from inferring a larger promise from a small change. -->

-

## 8. Follow-up

<!-- Leave empty when there is no concrete next item. Link issue, decision,
     roadmap track or next PR when there is one. -->

-
