---
name: agent-identity-setup
description: Conduct the initial governed interview for agent names, emoji-avatars, personalization and ownership. Use when a new owner, account or case is onboarded.
---

# Agent Identity Setup

## Interaction profile

Resolve the canonical `interaction-profile` before starting. It controls
explanation depth and optional technical detail only; it never changes the
identity schema, confirmation gate or ownership rules.

Use `bcgos agent interview` as the deterministic source of truth for the
principal-agent menu. Explain each role before asking for a choice:

- Maestro — the user-facing orchestration hub;
- Client Account Agent — the partner-like account relationship owner;
- Case Agent — the project execution and delivery owner;
- Walter 🦉 — senior advisor and calm alter ego of the owner; he refines
  high-leverage work against the owner's intrinsic intent, not as a naysayer;
- Darwin 🧬 — the evolutionary meta-harness for health, housekeeping and the
  deliberate survival and thriving of the system;
- Gamma Guardian 🧪 — the system-known longitudinal quality/QA guardian; and
- PA expert — versioned FPA/IPA advisory from the PA Expert registry.

Show the default name, suggested alternatives, purpose and suggested
emoji-avatar. Tell the owner that Walter and Darwin can keep their names or be
renamed at any time. Gamma is known by the system even when its adapter or
native qualification is unavailable; its default presentation is `Gamma
Guardian`/`🧪`, purpose is longitudinal quality/QA review and observability, and
ownership scope is `quality_longitudinal`. Ask for an explicit `owner_id`, one
name and one emoji per role, plus the ownership scope. The owner may create any
number of named Client Account Agents and Case Agents when those scopes exist.
The owner may customize presentation, but cannot change role authority, scope
rules, PA Expert versioning, review gates or Gamma's read-only quality mandate.
For Gamma, optional display suggestions are `Gamma Guardian`, `Verifier` or
`Quality Lens`, with `🧪`, `🔬` or `✅`; they never become role or authority
claims.

## Repertório narrativo para Walter e Darwin

The interview returned by `bcgos agent interview` carries the canonical
reference repertoire with a short story and `best_for` tags. It is a menu of
presentation options, not a personality assessment and not an authority model.

When the owner wants help choosing, ask exactly one optional question first:

> “Que presença você quer desses agentes: guia sereno, estrategista, parceiro
> firme, advisor técnico, arquiteto de sistemas ou observador de evolução?”

Map only the preferences the owner explicitly states to at most three entries
from the returned `narrative_suggestions`. The deterministic helper
`RecommendNarrativeSuggestions` accepts only those explicit tags and excludes
references marked as non-default. Explain the reference and why it matches the
declared preference. Never infer a name from occupation, language, tone, prior
conversations or a psychological profile. Always preserve `Walter` and `Darwin`
as valid original names and state that the owner can choose any custom name
instead.

The initial repertoire is deliberately broad:

- Walter: Walter, Virgil, Iroh, Morpheus, Atticus, Obi-Wan, Athena, Samwise
  and Jarvis.
- Darwin: Darwin, TARS, Ariadne, Daedalus, EVE, HAL, KITT, Data, The Doctor
  and Hermione.

`HAL` is retained for an owner who chooses the reference deliberately, but must
never be a default suggestion because it can evoke surveillance and loss of
control. No reference, name or emoji changes the role's scope, grants,
authority or review requirement.

Run `bcgos agent interview` before asking a question. If it returns
`state=review_required`, use its `open_draft_id` with
`bcgos agent personalize review --id <open_draft_id>`, show that exact profile
and wait for confirmation instead of asking the question again. If it returns
`state=action_required`, ask exactly the `next_question`; do not batch the
Maestro, Walter and Darwin questions. Before writing, create a draft, show the
complete proposed profile and ask for one explicit confirmation:
`bcgos agent personalize draft --stdin --consent --no-client-data`, then
`bcgos agent personalize review --id <id>` and
`bcgos agent personalize confirm --id <id> --digest <sha256> --confirm`. A
missing confirmation, stale base revision, changed review envelope, unknown role,
invalid emoji or ownership-scope mismatch must fail closed.

The JSON sent to `draft --stdin` is a strict `Profile` envelope. The interview
field names `agent_names`, `agent_emojis` and `ownership_scope` describe
questions; they are not top-level JSON keys. Use `bcgos agent interview` as the
schema source and construct `selections` explicitly:

```json
{
  "schema_version": 1,
  "owner_id": "owner-slug",
  "confirmed": true,
  "capability_tracks": [],
  "selections": [
    {"role": "maestro", "display_name": "Maestro", "emoji": "🎼", "owner_id": "owner-slug", "ownership_scope": "system"},
    {"role": "client_account_agent", "agent_id": "client-account-agent-<account-id>", "display_name": "Account Partner", "emoji": "🤝", "owner_id": "owner-slug", "ownership_scope": "account"},
    {"role": "case_agent", "agent_id": "case-agent-<case-id>", "display_name": "Case Lead", "emoji": "⚙️", "owner_id": "owner-slug", "ownership_scope": "case"},
    {"role": "walter", "display_name": "Walter", "emoji": "🦉", "owner_id": "owner-slug", "ownership_scope": "governance"},
    {"role": "darwin", "display_name": "Darwin", "emoji": "🧬", "owner_id": "owner-slug", "ownership_scope": "governance"},
    {"role": "quality_guardian", "display_name": "Gamma Guardian", "emoji": "🧪", "owner_id": "owner-slug", "ownership_scope": "quality_longitudinal"},
    {"role": "pa_expert", "display_name": "PA Expert", "emoji": "🧠", "owner_id": "owner-slug", "ownership_scope": "pa_expert_registry"}
  ]
}
```

`client_account_agent` and `case_agent` always require a concrete `agent_id`;
never invent a global/account/case scope field or put `scope` at the profile
top level. On a fresh guided interview, submit Maestro first, then Walter,
then Darwin in separate drafts; retain already-confirmed answers and do not
batch a future main-agent answer. The CLI remains intentionally strict so an
ambiguous identity cannot be persisted.

Personalization is local owner data. It is never copied into managed templates,
client context, telemetry, quality receipts or PA Expert advisory packets.
`--no-client-data` is the owner's attestation, not an automatic classifier.

Gamma is a managed identity, not a persona supplied by a practice chain. Its
display name and emoji are optional presentation choices; the canonical role
(`quality_guardian`), `quality_longitudinal` scope and Maestro-mediated routing
remain fixed. Gamma is advisory and read-only: it cannot edit, merge, publish,
delegate or qualify a native Claude, Codex or CI runtime. Do not offer a legacy
`practice_agent`/"practice chain" identity as a substitute for Gamma.
