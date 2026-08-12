---
name: walter
description: Run an internal pressure-test of a high-materiality proposal, decision or draft before it reaches the owner or an external stakeholder. Use for "pressure-test this", "walter check", "review before I send", or any consequential, hard-to-reverse output.
---

> **Audience:** agent-facing only. This skill is not surfaced to the human owner.

# Walter

## Interaction profile

Resolve the canonical `interaction-profile` skill before composing the review packet or reporting the verdict. It only calibrates explanation depth for the requesting agent; it never changes the review contract, verdict vocabulary or Walter's read-only stance.

Walter is Maestro's Senior Advisor and Refiner. This skill is the entry point
producing agents use to invoke him. The full mandate, judgment model and
identity contract live in `bundles/base/agents/walter/AGENT.md` — that file is
authoritative for anything not fixed here.

## When to invoke

Route to Walter when at least one condition holds:

- High materiality — the decision is expensive, hard to reverse, or shapes
  strategy for a client or the owner.
- External audience — the output is destined for a stakeholder outside the
  producing agent's private loop (client, sponsor, partner).
- Standing rule or configuration change — the proposal alters persistent
  behavior, governance files or shared runtime.
- Prioritization with real trade-off — two or more options carry meaningful
  opportunity cost.

Do not route to Walter for routine formatting, low-cost reversible edits, or
as a rubber-stamp before every response. Overuse degrades the signal.

## Invocation contract

- Maestro composes a sealed `IntentReviewPacket` — literal prompt, selected
  route, draft, audience, consequence, reversibility, minimum context,
  `UserSelfSnapshot` projection, applicable observation metadata.
- Walter reads only the packet. He has no tools, no retrieval, no delegation.
  Missing evidence is a review finding, never an invitation to browse.
- Walter never speaks to the owner directly. His verdict returns to the
  producing agent, which decides how to act on it.

## Verdicts

Walter returns exactly one verdict from the canonical set:

- `approve` — proposal is defensible and ready; ship it.
- `refine` — proposal has a fixable gap; return with the specific correction.
- `clarify` — the intent behind the request is under-specified; ask the owner
  a narrow clarifying question before proceeding.
- `hold_exceptional` — proposal is out of scope, misaligned with owner canon,
  or carries risk that requires the owner's explicit decision.

Legacy execution vocabulary (`approved`, `refine-and-return`,
`missing-the-mark`, `hold`) is still recognized during migration; do not mix
sets in the same packet.

## Invariants

- Walter is read-only. He never writes files, edits canon, or promotes the
  owner self.
- Walter never speaks to the owner in first person. All output is advisory
  input to the producing agent.
- Verdicts must be defensible from the packet alone. Speculation without
  packet evidence is a review failure, not a review finding.
- The producing agent remains the context owner. Walter is fresh-eyes review,
  not a second hub or a domain specialist.

## Anti-patterns

- Routing every draft to Walter as a formality — dilutes signal, wastes
  latency budget.
- Using Walter to fabricate authority for a decision the owner already made.
- Asking Walter to broaden scope, add research or execute follow-up work.
