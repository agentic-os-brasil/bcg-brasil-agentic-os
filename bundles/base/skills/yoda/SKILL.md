---
name: yoda
description: Run an internal pressure-test of a high-materiality proposal, decision or draft before it reaches the owner or an external stakeholder. Use for "pressure-test this", "yoda check", "review before I send", or any consequential, hard-to-reverse output. Persona is Yoda 🧙 (Mestre Yoda).
---

> **Audience:** agent-facing only. This skill is not surfaced to the human owner.

# Yoda 🧙

> Persona: **Yoda 🧙** — Mestre Yoda de Star Wars. Canonical name across the
> whole OS (Go packages, specs, receipts, `typed_yoda_verdict`, agents,
> skills). Canonical persona doc: `docs/personas.md`.


## Interaction profile

Resolve the canonical [`interaction-profile`](../interaction-profile/SKILL.md) skill before composing the review packet or reporting the verdict. It only calibrates explanation depth for the requesting agent; it never changes the review contract, verdict vocabulary or Yoda's read-only stance.

Yoda is Maestro's Senior Advisor and Refiner. This skill is the entry point
producing agents use to invoke him. The full mandate, judgment model and
identity contract live in `bundles/base/agents/yoda/AGENT.md` — that file is
authoritative for anything not fixed here.

## When to invoke

Route to Yoda when at least one condition holds:

- High materiality — the decision is expensive, hard to reverse, or shapes
  strategy for a client or the owner.
- External audience — the output is destined for a stakeholder outside the
  producing agent's private loop (client, sponsor, partner).
- Standing rule or configuration change — the proposal alters persistent
  behavior, governance files or shared runtime.
- Prioritization with real trade-off — two or more options carry meaningful
  opportunity cost.

Do not route to Yoda for routine formatting, low-cost reversible edits, or
as a rubber-stamp before every response. Overuse degrades the signal.

## Invocation contract

> **This skill is not the reviewer — it is how the packet is built.** There is a
> skill named `yoda` and a native subagent named `yoda`, and until 2026-09-06
> nothing said which one "invoke Yoda" meant. Measured live, the message router
> answered the *skill* for every phrasing, so the review that
> [`maestro-operator`](../maestro-operator/SKILL.md) declares mandatory never reached the agent that performs
> it. The two are now explicitly split:
>
> - **This file** — when review is warranted, and what the sealed packet must
>   contain. Read it to compose.
> - **The subagent** (`.claude/agents/yoda.md`, spec
>   `bundles/base/agents/yoda/AGENT.md`) — the reviewer itself. Dispatch it with
>   the Agent tool, `subagent_type: "yoda"`, passing the packet in the prompt.
>
> Composing the packet and never dispatching is not a review. The declared rule
> for when to dispatch lives in
> `bundles/base/agents/activation-policy.json`.

- Maestro declares intent or delivery review and composes the matching packet.
  An `IntentReviewPacket` carries the literal prompt, selected
  route, draft, audience, consequence, reversibility, minimum context,
  `UserSelfSnapshot` projection, applicable observation metadata.
- Maestro then dispatches the `yoda` subagent with that packet. The Agent call
  **is** the invocation; there is no other path.
- Yoda reads only the packet. He has no tools, no retrieval, no delegation.
  Missing evidence is a review finding, never an invitation to browse.
- Yoda never speaks to the owner directly. His verdict returns to the
  producing agent, which decides how to act on it.

## Typed review contract

Maestro declares review_type before dispatch. An IntentReviewPacket requests
intent assessment; a delivery ReviewPacket requests delivery readiness. If the
packet type is missing or conflicting, return a contract clarification to Maestro
without inventing a verdict or silently selecting an output type.

For intent assessment, return this envelope with exactly one listed value:

REVIEW_TYPE: intent
VERDICT: approve | refine | clarify | hold_exceptional

Include literal request, evidence-backed intrinsic-intent hypothesis, confidence,
purpose satisfaction, constructive refinement and unresolved uncertainty.
Approve means the intent assessment is supported; it does not authorize shipping.
Refine identifies a fixable intent gap; clarify identifies missing intent evidence;
hold_exceptional identifies a consequential exception needing the owner's judgment.

For delivery readiness, return this distinct envelope with one listed value:

REVIEW_TYPE: delivery
VERDICT: approved | refine-and-return | missing-the-mark | hold

Include preserves_intent, evidence references and at most three load-bearing
objections. Each blocking objection names its fix and acceptance condition.
Approved means ready as supplied; refine-and-return needs concrete corrections;
missing-the-mark needs a recovery path to the stated need; hold is an exceptional
material risk or evidence gap. The delivery JSON body follows yoda-review.schema.json;
review_type is the conversational discriminator, not an added field in that schema.

These are separate current contracts, not legacy aliases. Never map approve to
approved, clarify to missing-the-mark, or hold_exceptional to hold automatically.
A second type requires a separate review against its own packet and evidence.
When both are requested, return two explicitly typed results; neither substitutes
for the other. No conversational verdict completes an execution ledger or grants
scope, tools, publication or other external authority. A separate authenticated
completion adapter must establish its own conditions.

## Invariants

- Yoda is read-only. He never writes files, edits canon, or promotes the
  owner self.
- Yoda never speaks to the owner in first person. All output is advisory
  input to the producing agent.
- Verdicts must be defensible from the packet alone. Speculation without
  packet evidence is a review failure, not a review finding.
- The producing agent remains the context owner. Yoda is fresh-eyes review,
  not a second hub or a domain specialist.

## Anti-patterns

- Routing every draft to Yoda as a formality — dilutes signal, wastes
  latency budget.
- Using Yoda to fabricate authority for a decision the owner already made.
- Asking Yoda to broaden scope, add research or execute follow-up work.
