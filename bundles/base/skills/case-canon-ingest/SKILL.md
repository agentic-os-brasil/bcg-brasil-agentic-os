---
name: case-canon-ingest
description: Compile a document, interview synthesis, hypothesis, framework, or benchmark into the active case's canon layer as a frontmatter-indexed Markdown artifact. Builds persistent compiled knowledge per case so context survives session resets. Use whenever a document has been reviewed and its key insights need to be retained across sessions.
---

# Skill: case-canon-ingest

Resolve the canonical `interaction-profile` before starting. It controls verbosity during
extraction, but never changes classification requirements, provenance rules, or the
confirmation-before-save invariant.

## When to use

A document, interview, dataset, framework, or benchmark has been reviewed and its distilled
insights need to persist across sessions in the active case. The canon layer is the compiled
knowledge base for the case — not raw documents, but extracted, structured, reusable context.

Trigger phrases (invoke this skill when these appear):
- "ingere isso no canon" / "ingest this into canon"
- "adiciona ao canon do caso" / "add to case canon"
- "salva isso como canon" / "save this as canon"

Use for:
- Interview synthesis (key themes, verbatim quotes, constraints surfaced)
- Hypothesis distillation (hypothesis → evidence mapping)
- Framework or methodology reference (applied to this case)
- Benchmark or data reference (key numbers, sources, caveats)
- External research synthesis (approved public sources only — no confidential client bodies)

Do NOT use for:
- Structural decisions → use `decision-log-entry`
- Raw document storage → keep source pointer in `brain/sources/`, not body in canon
- Cross-case knowledge → each canon artifact is case-scoped; no cross-case lookup

---

## Canon artifact types

| Type | Use when |
|------|----------|
| `hypothesis` | A structured hypothesis with evidence mapping |
| `interview` | Synthesis of one or more stakeholder interviews |
| `data` | Key numbers, data sources, or dataset description |
| `framework` | A methodology or analytical framework applied to this case |
| `benchmark` | Competitive, industry, or historical benchmark reference |

Note: `decision` type is redirected to `decision-log-entry`.

---

## Sequence

### Step 1 — Confirm active case

Read `data/cases/.active` to get the current case-id.
If `.active` is absent or empty, stop and ask the owner to activate a case first
(`/case-agent-setup`).

Canon dir: `data/cases/<case-id>/brain/canon/`

Create the directory if it does not exist.

### Step 2 — Determine artifact slug and type

Ask the owner (or infer from context):
- **Type** — from the canonical list above
- **Title** — short descriptive title (used in slug and frontmatter)
- **Source** — where the content came from (document name, interview participant role, URL, etc.)

Generate slug: `<type>-<kebab-title>.md` (e.g., `hypothesis-corretor-conversion-gap.md`)

Check if a file with this slug already exists. If yes, offer to update (append a new dated
section) rather than overwrite — preserve history.

### Step 3 — Extract distilled content

Work with the owner to produce the canon content:

**For `hypothesis`:**
- Core hypothesis statement (one sentence)
- Supporting evidence (bulleted, each with source reference)
- Counter-evidence / assumptions to test
- Status: `exploratory` | `supported` | `rejected`

**For `interview`:**
- Participant role (never name — role only, e.g. "Head of Distribution")
- Date (YYYY-MM-DD)
- Key themes (bulleted)
- Verbatim quotes (at most 3, in `> "..."` blocks — never paraphrase)
- Constraints or blockers surfaced
- Open questions raised

**For `data`:**
- Dataset / source name and version
- Key metrics (table or bulleted)
- Caveats and known gaps
- Validity window (if time-sensitive)

**For `framework`:**
- Framework name and origin
- Core logic (2–4 bullet summary)
- How applied to this case
- Key output or recommendation it supports

**For `benchmark`:**
- Benchmark name / source
- Key numbers (table preferred for ≥3 items)
- Applicability to this case
- Source URL and retrieval date

### Step 4 — Write frontmatter-indexed artifact

```markdown
---
title: <Title>
type: <type>
source: <source reference>
date: <YYYY-MM-DD>
status: active
case: <case-id>
tags: [<tag1>, <tag2>]
---

# <Title>

<distilled content per type template above>
```

### Step 5 — Show before saving

Show the complete artifact to the owner before writing:

```
Vou salvar esse canon artifact em brain/canon/<slug>:

<artifact>

OK?
```

On confirmation, write the file. On correction, adjust per feedback.

### Step 6 — Suggest next action

- If `type == hypothesis`: "Add this to the case brief's hypotheses section?"
- If `type == interview`: "Any constraints surfaced that should become tasks in brain/tasks/?"
- If `type == data`: "Flag any gaps as open questions in the case brief?"
- If `type == benchmark`: "Link this benchmark in the relevant deliverable?"

---

## Anti-patterns

❌ **Copy client document bodies** — extract insights only; source pointer goes in `brain/sources/`
❌ **Cross-case references** — each artifact is strictly scoped to its case
❌ **Skip frontmatter** — frontmatter is what makes the canon machine-readable across sessions
❌ **Save without confirming** — always show the owner before writing
❌ **Redirect decisions here** — structural decisions go to `decision-log-entry`
