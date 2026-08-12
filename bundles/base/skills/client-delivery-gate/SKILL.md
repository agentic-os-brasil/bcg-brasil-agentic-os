---
name: client-delivery-gate
description: Mandatory pre-delivery quality gate for any output going to a client or external stakeholder. Reviews through three analytical lenses — Kahneman (analytical solidity), Taleb (tail risk and fragility), and Ariely (framing integrity). Returns LIBERADO, LIBERADO COM RESSALVAS, or SEGURAR with specific concerns and suggested fixes. Does not rewrite output. Use before sending any deck, memo, analysis, or recommendation to a client.
---

# Skill: client-delivery-gate

Resolve the canonical `interaction-profile` before starting. It controls how findings are
presented (compact vs. verbose), but never changes the three-lens structure, verdict
categories, or the 🔴 SEGURAR hard-stop invariant.

## When to use

Trigger phrases (invoke this skill when these appear):
- "gate de entrega" / "delivery gate" / "passa pelo gate"
- "pode entregar?" / "ready to deliver?"

Before any output leaves the workspace and reaches a client, sponsor, or external stakeholder.
This gate is mandatory for:
- Slide decks and presentation sections
- Written memos, emails, or briefings
- Quantitative analyses with recommendations
- Hypotheses or conclusions presented as findings

Do NOT use as a substitute for good work — run this after the work is substantively done.
The gate reviews; it does not write.

---

## Three-lens review

### Lens 1 — Kahneman (Analytical Solidity)

Checks for:
- **Availability bias**: are conclusions over-weighted by recent or memorable data?
- **Anchoring**: are numbers anchored to an arbitrary reference without justification?
- **Confirmation bias**: does the analysis test only hypotheses that confirm the thesis?
- **Precision theater**: is false precision masking genuine uncertainty (e.g., `47.3%` when `~50%` is honest)?
- **MECE compliance**: are the recommendations mutually exclusive and collectively exhaustive?

### Lens 2 — Taleb (Tail Risk and Fragility)

Checks for:
- **Hidden assumptions**: what must be true for the recommendation to hold? Are these stated?
- **Fragility**: does the recommendation break under a plausible stress scenario?
- **Narrative fallacy**: is a causal story being told where only correlation exists?
- **Missing tail scenarios**: are low-probability, high-impact failure modes acknowledged?
- **Overfitted specificity**: is the recommendation so tailored to current conditions that it fails under small changes?

### Lens 3 — Ariely (Framing Integrity)

Checks for:
- **Framing effect**: would the same data presented differently lead to a different conclusion?
- **Decoy effects**: are choices presented in a way that steers the audience toward a predetermined option?
- **Anchoring in presentation**: does the narrative lead with a number or comparison that unfairly anchors interpretation?
- **Omission**: is material information that would change the audience's assessment left out?
- **Over-commitment language**: does the output commit beyond what the evidence supports (e.g., "will" vs. "may")?

---

## Sequence

### Step 1 — Scope the review

Confirm with the owner:
- What is being reviewed? (deck, section, memo, analysis)
- Who is the audience? (C-suite, working team, sponsor, regulator)
- What decision does the output support?

This scoping is not a gate — answer these from context if obvious.

### Step 2 — Run the three lenses

For each lens, produce a **findings list** with:
- **Issue**: what the concern is
- **Location**: where in the output (section title, slide number, specific claim)
- **Risk**: why it matters for this audience
- **Suggested fix**: what change would address it (do not rewrite — suggest only)

Issues may be:
- 🔴 **Blocker** — would materially mislead the audience or damage credibility
- 🟡 **Caution** — reduces quality or leaves the team exposed; worth addressing
- ⬜ **Note** — minor, addressable on next pass if time allows

### Step 3 — Render verdict

After all three lenses:

```
## Verdict

🟢 LIBERADO
All three lenses clear. No blockers or cautions found. Ready to deliver.

— or —

🟡 LIBERADO COM RESSALVAS
Ready to deliver after addressing: [list cautions and their fixes].
Blockers: none. Cautions: [N].

— or —

🔴 SEGURAR
Do not deliver until blockers are resolved: [list blockers].
[N] blocker(s) found. [N] caution(s) also noted.
```

### Step 4 — Summarize for the owner

Return:
- Verdict (🟢 / 🟡 / 🔴)
- Per-lens findings (only non-trivial items)
- Ordered action list (blockers first, then cautions)
- Estimated effort to resolve (rough: <30min / ~1h / >1h per item)

---

## Invariants

- The gate never rewrites output — it identifies concerns and suggests fixes only.
- The gate is case-scoped: it reviews only the artifact presented, not prior work or other cases.
- 🔴 SEGURAR is a hard stop — the owner must explicitly override and accept responsibility before delivery proceeds.
- Public data and assumptions must be declared; gate flags undeclared assumptions as blockers.
- The gate does not replace the case owner's judgment — it surfaces blind spots.

---

## Anti-patterns

❌ **Rewrite the output** — suggest only; rewriting belongs to the owner
❌ **Approve under time pressure** — a 🔴 SEGURAR under a deadline is still 🔴 SEGURAR
❌ **Skip a lens because the output looks good** — all three lenses run every time
❌ **Overload with notes** — only surface issues that matter for this audience; filter noise
❌ **Run the gate on unfinished work** — gate assumes the work is substantively complete
