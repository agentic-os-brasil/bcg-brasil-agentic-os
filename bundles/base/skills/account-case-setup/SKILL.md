---
name: account-case-setup
description: Create the right Maestro workspace context for a client account and its project Case Agent. Use when an owner starts a client engagement, needs to set up a project, name its key agents, or wants a simple internal project without technical configuration.
---

# Account Case Setup

## Interaction profile

Resolve the canonical [`interaction-profile`](../interaction-profile/SKILL.md) skill before responding. It adjusts explanation depth and language only; it never changes scope, authority, or the confirmation gates.

Turn a client engagement into a useful working setup, not a taxonomy exercise. The owner chooses names and emoji-avatars; Maestro keeps the roles clear.

## Choose the right starting point

For a client engagement, recommend the natural order:

```text
Client Account Agent → Case Agent → first useful delivery → checkpoint
```

The Client Account Agent is the partner-like steward of the relationship, stakeholders, strategic context and account narrative. The Case Agent executes the particular project. For a low-stakes internal task, a direct Case Agent is appropriate; do not create account structure merely for ceremony.

## Canonical locations

Every client account — and every internal (non-client) account used the same way —
lives at `brain/accounts/<account-id>/`:

- `brain/accounts/<account-id>/<account-id>.md` — the account brief, and the
  account's only file: relationship context, stakeholders, strategic view,
  intended use, and a `## Cases` section linking every case that belongs to it.
  Named after the account itself, not a bare `account.md`, so every account's
  brief has a distinct filename across the whole brain — the same
  folder-name-as-filename pattern used for the curated companion pages
  elsewhere in the brain (`craft/craft.md`, etc.). Unlike those, an account has
  no separate generated index at all: `accounts_index.md` links to this file
  directly, and this page is never touched by the compiler — the `## Cases`
  list is entirely hand-maintained ([`case-agent-setup`](../case-agent-setup/SKILL.md) adds a line here when it
  creates a case). This is the only place account-level narrative is written;
  it is never duplicated into a case.
- `brain/accounts/<account-id>/agent.json` — the Client Account Agent's identity for
  this specific account (name, emoji, `owner_id`, `account_id`), written by
  `$agent-identity-setup`. One identity per account, not a shared global block.
- `brain/accounts/<account-id>/cases/<case-id>/` — every case (project) that belongs to
  this account, each with its own identity and `brain/` (see [`case-agent-setup`](../case-agent-setup/SKILL.md)).

An account groups one or more cases over time; a case never exists outside an account.
For internal, non-client work, use (or create) an internal account rather than
skipping the account tier — e.g. `brain/accounts/<team-or-initiative-slug>/`.

## Create one clear working context at a time

1. Confirmar o espaço de trabalho atual do Maestro e ajustar o tom e o nível de detalhe ao perfil do usuário.
2. Ask for the account or project name, what success looks like and the first useful outcome. Do not request a schema, agent ID or a folder plan.
3. Explain the proposed pair in everyday language, including their suggested names and emoji-avatars. Offer customization through `$agent-identity-setup` when the owner wants it. Persist the account identity to `brain/accounts/<account-id>/agent.json` and the case identity to `brain/accounts/<account-id>/cases/<case-id>/agent.json` — each is its own instance, never a shared role block.
4. For an account, capture only the authorized relationship context: stakeholders, important concepts, strategic view and the intended use of the context, writing it to `brain/accounts/<account-id>/<account-id>.md`. Do not import client material, crawl SharePoint or infer a cross-project mandate.
5. For a Case Agent, use `$case-agent-setup` to create the reviewed project brief, first plan and its next action under `brain/accounts/<account-id>/cases/<case-id>/`. Keep the Case Agent within its workspace and make promotion back to the account periodic and deliberate.
6. End with one small delivery or checkpoint. A complete setup is useful only when it leads to work.

## Keep the roles honest

PA Experts (especialistas de área do BCG) são consultores transversais em conhecimento funcional e de indústria. Não são agentes de projeto, não possuem o contexto do cliente e são consultados de forma continuada quando a perspectiva deles agrega valor.

## Definição de conclusão

O setup está completo quando o owner tem um contexto de trabalho nomeado, sabe qual é o próximo passo concreto e recebeu uma primeira entrega ou checkpoint útil.

## Contrato de página do brain

Toda página escrita em `brain/` precisa do frontmatter definido em
`bundles/base/brain-contract.md` — `id`, `title`, `summary`, `type`, `scope`, `status`,
`sensitivity`, `updated`. Leia esse arquivo antes de gravar e escreva o bloco junto com a
página, nunca depois.

Uma página sem esse bloco não aparece no índice do brain e não recebe backlinks: o
trabalho fica gravado e invisível.
