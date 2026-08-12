---
name: decision-log-entry
description: Transcribe a free-form decision (spoken or written) into a structured D-NNN entry in the active case decision log. Preserves verbatim rationale, infers type/decisor/linked specs, and proposes a review date. Use whenever a methodology, architecture, stakeholder, or override decision needs to be captured before it escapes.
---

# Skill: decision-log-entry

Resolve the canonical `interaction-profile` before starting. It controls verbosity and
confirmation style during capture, but never changes the D-NNN format, verbatim-quote
invariant, or save-confirmation requirement.

## When to use

A decision was just made — methodology, architecture, stakeholder, override — and needs to be
recorded in the case decision log before it escapes. The owner usually speaks freely;
this skill formats the entry.

Explicit trigger phrases (invoke this skill when these appear):
- "registra essa decisão" / "record this decision"
- "decision-log: ..."
- "anota no decision-log" / "log this decision"
- "isso vira D-NNN" / "this becomes D-NNN"
- Statement ending with "isso é decisão" / "that's the decision"

Do NOT use for:
- Substantive spec amendment → escalate to case owner + edit the spec
- Open premise / unresolved question → capture in `brain/projects/<brief>.md` open questions section
- Generic note capture → use `case-canon-ingest`

---

## Inputs

- **`raw`** (required) — free-form decision text, ideally verbatim from the owner
- **`type`** (optional) — if not stated, infer from canonical categories:
  - `spec-approved` · spec approved for execution
  - `spec-amended` · mid-sprint change
  - `override-proposed` / `override-approved` · change to locked decisions
  - `methodology` · methodological decision
  - `architecture` · architecture/contract decision
  - `stakeholder` · decision about a relationship or person
  - `pending` · still open, will be locked when resolved

---

## Sequence

### Step 1 — Locate the active case decision log

Read `data/cases/.active` to get the current case-id.
If `.active` is absent or empty, stop and ask the owner to activate a case first
(`/case-agent-setup`).

Decision log path: `data/cases/<case-id>/brain/decisions/decision-log.md`

If the file does not exist, create it with a minimal header:
```markdown
# Decision Log — <case-id>

<!-- Entries appended chronologically. Latest at bottom. -->
```

### Step 2 — Determine next D-NNN

```bash
grep -E "^## D-[0-9]+" data/cases/<case-id>/brain/decisions/decision-log.md | tail -1
```

Take the highest number found and add 1. Format: `D-NNN` zero-padded to 3 digits.
If no entries exist, start at `D-001`.

### Step 3 — Parse raw into entry fields

**Required fields:**

| Field | How to infer |
|-------|-------------|
| `id` | Next D-NNN |
| `date` | Today in `YYYY-MM-DD` (unless owner cites another date explicitly) |
| `type` | From canonical list. Infer if not stated. |
| `title` | <50 chars, imperative. Extract from raw — do not invent. |
| `Decisor` | Default to the project owner. List others if cited. |
| `Context` | Why the decision needed to be made. Verbatim when possible. |
| `Decisão` | WHAT was decided. **Verbatim** when there is a literal quote — do not paraphrase. |
| `Racional` | Why this decision. Verbatim or minimal synthesis of raw. |
| `Linked` | Spec cited, locked decision, external ref. List refs found in raw. |

**Optional but recommended:**

| Field | How to infer |
|-------|-------------|
| `Review (target N days)` | Default 7 business days. `methodology`/`architecture` → 14d. `stakeholder` → 30d. |

### Step 4 — Preserve verbatim (anti-paraphrase rule)

If raw contains quotes or characteristic phrasing from the owner or a stakeholder:
- Keep literally with `> "phrase"` or `**"phrase"**` in the `Decisão` or `Quote canônico` field
- **Do not paraphrase.** Paraphrase is failure mode #1.

### Step 5 — Canonical format

```markdown
## D-NNN · YYYY-MM-DD · <type> · <short title>
- **Decisor:** <who>
- **Context:** <why the decision was needed>
- **Decisão:** <what was decided — verbatim when possible>
- **Racional:** <why this decision>
- **Linked:** <refs: spec / locked-decision / external>
- **Review (target YYYY-MM-DD):** pending
```

If there is a canonical quote:
```markdown
- **Quote canônico:** > "<literal phrase>"
```

If a new stakeholder was introduced:
```markdown
- **Stakeholder novo registrado:** <name>, <role>
```

### Step 6 — Show before saving

Show the formatted entry to the owner before writing:

```
Vou adicionar essa entry no decision-log:

## D-NNN · ...
<entry>

OK?
```

On confirmation, append the entry to the decision log file (maintain chronological order —
append at end with a blank line separator). On correction, adjust per feedback.

### Step 7 — Suggest next action

Always close with 1–2 concrete follow-up questions:
- If `type == spec-approved`: "Update the locked decisions file too?"
- If `type == architecture` + cross-team impact: "Worth communicating to the team via a doc PR?"
- If `type == override-proposed`: "Who needs to approve before it becomes locked?"
- If `type == stakeholder`: "Update the stakeholder map in the case brief?"

---

## Anti-patterns

❌ **Paraphrase when a literal quote exists** — failure mode #1. Verbatim or nothing.
❌ **Invent a field** — if raw has no explicit rationale, leave `Racional: <pendente>` and ask.
❌ **Assume decisor without evidence** — do not default to owner if context suggests a client or team member.
❌ **Skip review date** — entries without review dates become forgotten. Always propose a target.
❌ **Save without confirming** — always show the owner before writing to file.
