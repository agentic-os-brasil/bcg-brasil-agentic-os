<!--
  MAKE THE CHANGE EASY TO TRUST.
  Delete guidance comments before requesting review. State only observed facts:
  no imagined checks, capabilities or pilot readiness.
-->

# ✨ What changes for people?

<!-- In 2–3 sentences: the problem, the outcome and who benefits. Lead with the
     user or operator impact — not a file-by-file account. -->

## 🎯 Outcome at a glance

| Before | After | Why it matters |
| --- | --- | --- |
|  |  |  |

## 🧩 What is in this PR

### Delivered

- [ ] **Capability** — what now works and how it is experienced.
- [ ] **Guardrail** — what unsafe, confusing or incorrect behavior is prevented.

### Deliberate choices

<!-- Only choices that a reviewer needs to evaluate. Link the decision or spec
     that governs the change when one exists. -->

- **Choice** — why this was the smallest safe path.

## 🗺️ Review route

1. Start with `specs/NNN-...` or the governing decision — what must remain true.
2. Review the named implementation surface — how the outcome is achieved.
3. Confirm the named test or command — what proves the observable behavior.

## 🧪 Evidence, not optimism

| Evidence | Result | Notes |
| --- | --- | --- |
| Targeted test or manual check | ⬜ pass / ⬜ not applicable |  |
| `go run ./dev/harness validate` | ⬜ pass / ⬜ not run |  |
| `go run ./dev/harness validate --full` | ⬜ pass / ⬜ not run |  |
| CI — Windows, macOS, Linux | ⬜ pending / ⬜ pass / ⬜ unavailable |  |

### Observable behavior protected

<!-- Name the test, characterization or manual scenario. For docs/mechanical
     work, write “Not applicable — <reason>”. -->

-

## 🛡️ Contracts, privacy and portability

### Governing contract

- Decision: `NONE` / `ABCD` — consequence:
- Spec: `NONE` / `specs/NNN-...` — consequence:

### Data boundary

- [ ] No client data, personal data, credentials, conversations or real memory is included.
- [ ] Managed core remains separate from owner-local and workspace-local data.
- [ ] New writes are scoped, non-destructive and recoverable.
- [ ] Telemetry/receipts contain only the allowed metadata or typed vocabulary.
- Exception (if any) and rationale:

### Runtime truth

| Runtime | State | What this PR proves |
| --- | --- | --- |
| Claude | ⬜ native / ⬜ emulated / ⬜ degraded / ⬜ unavailable |  |
| Codex | ⬜ native / ⬜ emulated / ⬜ degraded / ⬜ unavailable |  |

- Shared contract or intentional difference:

## 🚫 Explicitly not included

<!-- Name adjacent expectations still outside this PR. This is part of the
     product contract, not a weakness. -->

-

## ➡️ Follow-up

<!-- Link a concrete next issue, decision, roadmap item or PR. Leave empty if
     there is no real follow-up. -->

-
