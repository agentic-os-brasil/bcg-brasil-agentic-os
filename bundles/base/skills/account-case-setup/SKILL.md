---
name: account-case-setup
description: Create the right Maestro workspace context for a client account and its project Case Agent. Use when an owner starts a client engagement, needs to set up a project, name its key agents, or wants a simple internal project without technical configuration.
---

# Account Case Setup

## MCP first-case boundary

For the first case in the current admitted workspace, use
[case-agent-setup](../case-agent-setup/SKILL.md): inspect current setup, review its
canonical registration, explicitly save the full brief through brain operations
and finish with a checkpoint only when work was requested. Reuse returned
bindings, paths and revisions; never hand-write an identity or account marker.
The workspace alias follows that same owner, not a second workflow.

This first-case bridge does not deliver the broader account narrative,
relationship history, cross-case recall/promotion or arbitrary agent hiring.
Keep those effects pending unless their separate owning capability is actually
available and explicitly requested. A local HMAC-authenticated registration is
instance integrity, not release signing, runtime activation or a tool grant.

Before owner-state file operations, follow [admitted owner-state paths](../interaction-profile/resources/owner-paths.md). Owner `brain/` paths are logical; confirmed case/workspace-local paths retain their scope.


## Interaction profile

Resolve the canonical [`interaction-profile`](../interaction-profile/SKILL.md) skill before responding. It adjusts explanation depth and language only; it never changes scope, authority, or the confirmation gates.

Turn a client engagement into a useful working setup, not a taxonomy exercise. The owner chooses names and emoji-avatars; Maestro keeps the roles clear.

## Choose the right starting point

For a client engagement, recommend the natural order:

```text
Client Account Agent → Case Agent → first useful delivery → checkpoint
```

The Client Account Agent is the partner-like steward of the relationship, stakeholders, strategic context and account narrative. The Case Agent executes the particular project. For a low-stakes internal task, a direct Case Agent is appropriate; do not create account structure merely for ceremony.

## Account narrative locations are a separate scope

A client account with a separately authorized account narrative lives at
`brain/accounts/<account-id>/`:

- `brain/accounts/<account-id>/<account-id>.md` — the account brief, and the
  account's only file: relationship context, stakeholders, strategic view,
  intended use, and a `## Cases` section linking every case that belongs to it.
  Named after the account itself, not a bare `account.md`, so every account's
  brief has a distinct filename across the whole brain — the same
  folder-name-as-filename pattern used for the curated companion pages
  elsewhere in the brain (`craft/craft.md`, etc.). Unlike those, an account has
  no separate generated index at all: `accounts_index.md` links to this file
  directly, and this page is never touched by the compiler — the `## Cases`
  list belongs to the separately reviewed account narrative; first-case setup
  does not append or overwrite it automatically. This is the account-level narrative;
  it is never duplicated into a case.
- The exact Client Account Agent registration belongs to the canonical local
  scaffold owner. Use its returned identity/reference; these narrative paths
  are not a request to create `agent.json` or to choose a role/parent manually.
- `brain/accounts/<account-id>/cases/<case-id>/` — the client case's authored
  pages, including its root brief (see [`case-agent-setup`](../case-agent-setup/SKILL.md)).
  Use the canonical registration reference; do not create another nested brain
  or an identity file from this narrative layout.

A client case has its exact immutable account relation. Direct internal work is
also valid: the Case Agent has workspace scope, no account relation or account
pair markers, and a default brief at `projects/<workspace-id>.md`. Do not invent
an internal account for ceremony. An existing exact binding wins without
renaming; this first-case unit does not silently rebind it to another mode/case.

## Create one clear working context at a time

1. Confirmar o espaço de trabalho atual do Maestro e ajustar o tom e o nível de detalhe ao perfil do usuário.
2. Ask for the account or project name, what success looks like and the first useful outcome. Do not request a schema, agent ID or a folder plan.
3. Explain the client-account/case relation, or the direct internal Case choice, in everyday language. Keep optional names/avatars as a draft until their owning reviewed capability is available. First-case registration uses the admitted setup route and its native review, not direct identity files or a generic scaffold request.
4. If an account narrative is separately requested, draft only authorized relationship context: stakeholders, concepts, strategic view and intended use. Its persistence needs its own admitted operation/review; the first-case registration is not proof that this broader account workflow is delivered. Do not import client material, crawl SharePoint or infer a cross-project mandate.
5. Use `$case-agent-setup` for the complete reviewed project brief and first plan at the returned destination. New client cases default to `accounts/<account-id>/cases/<workspace-id>/<workspace-id>.md` relative to the workspace brain; internal projects use `projects/<workspace-id>.md`. Existing exact destinations win. No automatic promotion back to account context occurs.
6. End with the actual saved brief and next owner/step. Create a checkpoint only if requested/accepted, using the setup owner's stable work identity; do not duplicate work on a retry. Report pending effects rather than declaring a fictional transaction complete.

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
