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
  deliberate survival and thriving of the system; and
- PA expert — versioned FPA/IPA advisory from the PA Expert registry.

Show the default name, suggested alternatives, purpose and suggested
emoji-avatar. Tell the owner that Walter and Darwin can keep their names or be
renamed at any time. Ask for an explicit `owner_id`, one name and one emoji per
role, plus the ownership scope. The owner may create any number of named Client
Account Agents and Case Agents when those scopes exist. The owner may customize
presentation, but cannot change role authority, scope rules, PA Expert
versioning or review gates.

Before writing, show the complete proposed profile and ask for one explicit
confirmation. Persist only the confirmed strict JSON profile through
`bcgos agent personalize --stdin`. A missing confirmation, unknown role,
invalid emoji or ownership-scope mismatch must fail closed.

Personalization is local owner data. It is never copied into managed templates,
client context, telemetry or PA Expert advisory packets.
