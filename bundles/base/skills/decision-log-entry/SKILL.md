---
name: decision-log-entry
description: Register a case decision in the active case decision log. Use when the owner says "register this decision", "log this", "decision:", "we decided", or describes a structural choice (methodology, architecture, stakeholder, scope). Does not write without confirmation.
---

# Decision Log Entry

Resolve the canonical `interaction-profile` before proceeding. It governs
explanation depth and disclosure; it never changes the confirmation gate or
invariants below.

## Purpose

Write a single structured D-NNN entry to the active case decision log. This
skill captures structural choices — methodology, architecture, stakeholder,
scope — so they survive session boundaries and are reviewable on a fixed cadence.

## Trigger phrases

- "register this decision"
- "log this decision"
- "decision:"
- "we decided"
- "anota essa decisão"
- "registra essa decisão"
- "isso é decisão"

## Method

### 1. Resolve active case

Read `data/cases/.active`.

If the file is absent or empty, stop and return:

> Nenhum caso ativo. Use `/bcg-case-kickoff` para iniciar um caso primeiro.

Never infer a case from path fragments, session history, or prompt context.

### 2. Locate the decision log

Target path: `data/cases/<case-id>/brain/decisions/decision-log.md`

If the file does not exist, create it with the header:

```
# Decision Log — <case-id>
```

### 3. Determine next D-NNN

Grep the file for lines matching `## D-NNN`. Extract the highest NNN found,
increment by one, and zero-pad to three digits. If no entries exist, start
at D-001.

### 4. Parse the owner's free speech

Map the input to the following fields:

| Field | Rules |
|---|---|
| `id` | D-NNN zero-padded to three digits |
| `date` | today YYYY-MM-DD |
| `type` | one of `methodology`, `architecture`, `stakeholder`, `scope`, `pending` |
| `title` | imperative phrase under 50 characters |
| `Context` | why the decision was necessary |
| `Decision` | what was decided — verbatim when a literal quote is present |
| `Rationale` | why this option was chosen |
| `Linked` | specs, PRs, docs explicitly referenced in the input |
| `Review target` | today +7d default; +14d for `methodology` or `architecture` |

If `type` or `title` cannot be determined from the input, ask the owner to
clarify before continuing. Never infer a type silently.

### 5. Show the formatted entry and confirm

Present the entry exactly as it will be appended and ask:

> Confirm this entry? (yes / edit / cancel)

Do not write anything until the owner confirms. On "edit", collect the
correction and present the revised entry again. On "cancel", discard and stop.

### 6. Append the confirmed entry

Append to `data/cases/<case-id>/brain/decisions/decision-log.md`:

```markdown
## D-NNN · YYYY-MM-DD · <type> · <title>
- **Context:** ...
- **Decision:** ...
- **Rationale:** ...
- **Linked:** ...
- **Review target:** YYYY-MM-DD
```

Add a blank line before the entry if the file is non-empty.

## Invariants

- Never paraphrase when the owner provides a literal quote — preserve verbatim.
- Never save without explicit owner confirmation.
- Never infer the active case from anything other than `data/cases/.active`.
- Never create an entry without both `type` and `title`.

## Error handling

| Condition | Response |
|---|---|
| `data/cases/.active` absent or empty | "Nenhum caso ativo. Use `/bcg-case-kickoff` para iniciar um caso primeiro." |
| `type` not determinable | Ask the owner to specify one of the five allowed types. |
| `title` exceeds 50 characters | Propose a shortened version and confirm before accepting. |
