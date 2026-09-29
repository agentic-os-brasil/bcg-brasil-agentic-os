---
name: agent-identity-setup
description: Conduct the initial governed interview for agent names, emoji-avatars, personalization and ownership. Use when a new owner, account or case is onboarded.
---

# Agent Identity Setup

## Interaction profile

Resolve the canonical [`interaction-profile`](../interaction-profile/SKILL.md) before starting. It controls
explanation depth and optional technical detail only; it never changes the
identity schema, confirmation gate or ownership rules.

Conduct the principal-agent interview conversationally. Explain each role
before asking for a choice:

- Maestro — the user-facing orchestration hub;
- Client Account Agent — the partner-like account relationship owner;
- Case Agent — the project execution and delivery owner;
- o revisor de qualidade do Maestro — gate interno de revisão e consistência;
- o monitor de saúde do sistema — mantém o ambiente operacional estável e alinhado; e
- o especialista de indústria — referência de advisory FPA/IPA por setor, versionada.

Show the default name, suggested alternatives, purpose and suggested
emoji-avatar. Ask the owner for an explicit `owner_id`, one name and one emoji
per role, plus the ownership scope. The owner may customize presentation, but
cannot change role authority, scope rules, industry-specialist versioning or review gates.

**Default naming for per-instance roles.** The suggested default name is
literal, not thematic: `agente_<account-id>` for the Client Account Agent and
`agente_<case-slug>` for the Case Agent (e.g. `agente_acme`,
`agente_diagnostico-2026`) — never an invented nickname or metaphor. Use a
neutral emoji per tier (e.g. 🏢 for accounts, 📁 for cases) unless the owner
asks for something else. This is a default suggestion, always shown for
confirmation like any other proposed identity — the owner can rename to
anything at any time.

Before writing, show the complete proposed profile and ask for one explicit
confirmation. A missing confirmation, unknown role, invalid emoji or
ownership-scope mismatch cancels the operation: do not write anything.

Persistence depends on the role's ownership scope:

- **Global roles** (Maestro, o revisor de qualidade, o monitor de saúde do sistema,
  o especialista de indústria) — exactly one identity ever, regardless of how many
  accounts or cases exist. Persist to `brain/owner/agents.json` (create the file
  if absent, overwrite the matching role block otherwise).
- **Per-instance roles** (Client Account Agent, Case Agent) — one identity per
  specific account or case, never a shared block. A Client Account Agent's identity
  is `{name, emoji, owner_id, role: "client_account_agent", account_id}`, written to
  `brain/accounts/<account-id>/agent.json`. A Case Agent's identity is
  `{name, emoji, owner_id, role: "case_agent", account_id, case_id}`, written to
  `brain/accounts/<account-id>/cases/<case-id>/agent.json`. Running this interview
  again for a *different* account or case never overwrites another instance's file —
  each account and each case keeps its own name and emoji for the life of that
  account or case.

Personalization is local owner data. It is never copied into managed templates,
client context, telemetry or industry-specialist advisory packets.
