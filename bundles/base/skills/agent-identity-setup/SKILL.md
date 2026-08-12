---
name: agent-identity-setup
description: Conduct the initial governed interview for agent names, emoji-avatars, personalization and ownership. Use when a new owner, account or case is onboarded.
---

# Agent Identity Setup

## Interaction profile

Resolve the canonical `interaction-profile` before starting. It controls
explanation depth and optional technical detail only; it never changes the
identity schema, confirmation gate or ownership rules.

Conduct the principal-agent interview conversationally. Explain each role
before asking for a choice:

- Maestro — the user-facing orchestration hub;
- Client Account Agent — the partner-like account relationship owner;
- Case Agent — the project execution and delivery owner;
- Walter — internal pressure-test and review gate;
- Darwin 🧬 — governance, drift and bounded system-maintenance surgeon; and
- PA expert — versioned FPA/IPA advisory from the PA Expert registry.

Show the default name, suggested alternatives, purpose and suggested
emoji-avatar. Ask the owner for an explicit `owner_id`, one name and one emoji
per role, plus the ownership scope. The owner may customize presentation, but
cannot change role authority, scope rules, PA Expert versioning or review gates.

Before writing, show the complete proposed profile and ask for one explicit
confirmation. Persist only the confirmed strict JSON profile by writing it to
`data/profile/agents.json` (create the file if absent, overwrite the matching
role block otherwise). A missing confirmation, unknown role, invalid emoji or
ownership-scope mismatch must fail closed: do not write the file.

Personalization is local owner data. It is never copied into managed templates,
client context, telemetry or PA Expert advisory packets.
