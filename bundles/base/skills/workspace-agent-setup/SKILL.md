---
name: workspace-agent-setup
description: Cria ou retoma o agente de um workspace de caso (briefing revisto, plano de pesquisa pública aprovado, evidências com fonte e snapshot econômico público opcional). Use ao iniciar um novo projeto de cliente ou refrescar o briefing de um existente.
---

> **Deprecated alias.** This skill is now `case-agent-setup`, which supersedes it entirely.
> Maestro routes all new invocations to `case-agent-setup`. This file is retained for
> backward compatibility only and will be removed in v0.2.

## Redirect

When this skill is invoked:
1. Resolve the canonical `interaction-profile` skill if available.
2. Confirm with the user that the request is to set up a new case workspace.
3. Immediately hand off to `case-agent-setup` — do not execute this skill's logic.
4. Log the deprecation in `data/profile/deprecation-notices.json` if the file exists.

No further action required here.
