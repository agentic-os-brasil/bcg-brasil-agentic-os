---
name: record-decision
description: Record durable product, architecture, security, data, runtime or development decisions in the BCG Brasil Agentic OS project decision log. Use when a choice changes a lasting contract or supersedes a prior decision; do not use for task status, release notes, client context, trivial implementation choices or mechanical edits.
---

# Record Decision

Capture a durable choice as an append-only four-letter entry and verify log integrity deterministically.

## Workflow

1. Confirm the choice is durable enough for the decision log. If it is status, a task, release information or a reversible local detail, use the appropriate artifact instead.
2. Exclude secrets, personal data, client-identifying information and case content.
3. Read `docs/decisions/decision-log.md` and related specs.
4. Choose a memorable code containing exactly four uppercase letters. The code is only a permanent key; never infer chronology, priority or semantics from it.
5. Run `go run ./dev/harness decision available ABCD`, replacing `ABCD` with the proposed code.
6. Append an entry using the exact field set below.

```markdown
## ABCD - Concise decision title

- Date: YYYY-MM-DD
- Status: proposed|accepted|superseded|rejected
- Owner: accountable decision owner
- Context: why a durable choice is required
- Decision: the chosen contract or direction
- Consequences: important tradeoffs and follow-up implications
- Refs: specs, tests, issues or artifacts
- Supersedes: prior four-letter code or none
```

7. Run `go run ./dev/harness decision check`.
8. If another branch claimed the same code, choose another and resolve the collision before merge.
9. When changing an accepted decision, append a new entry with `Supersedes`; never rename, reuse or rewrite the old code.
10. Run `go run ./dev/harness validate --full` when the decision accompanies implementation.

Keep the log sparse. Its value comes from durable choices remaining visible above implementation noise.
