---
name: maestro-environment-setup
description: Prepare a post-install Maestro workspace conversationally. Use after the Maestro installer has completed, when an owner wants to prepare a workspace, connect Claude or Codex, or make sure the local working environment is ready without following technical setup steps.
---

# Maestro Environment Setup

Prepare one new local workspace and its normal working environment after Maestro has been installed. Keep the mechanics behind Maestro: the owner receives one clear confirmation and a short outcome, not a terminal checklist. For first installation, update, rollback or installer repair, hand off to `$maestro-setup-update`.

Resolve the canonical `interaction-profile` skill before responding. Ajustar o tom e o nível de detalhe da resposta ao perfil do usuário antes de apresentar a preparação. Isso afeta apenas a linguagem e a quantidade de detalhe opcional, nunca a transação de configuração, o limite de responsabilidade ou a confirmação.

## Prepare the environment

1. Verificar o ambiente de execução do Maestro e confirmar que o espaço de trabalho está acessível.
2. Ask one short question: **“Posso preparar este espaço do Maestro agora?”** Explain that this creates or refreshes only the local workspace and its integrations; it does not read prior files, publish anything or change another workspace.
3. After agreement, run the installed `setup apply` route once with the active workspace, runtime and executable. This is the deterministic consolidation: it initializes the workspace if needed, installs the runtime projection and hooks, verifies the local projection and reuses the local setup grant on later checkups.
4. Inspect Darwin's user-level maintenance state without trying to replace it. The visual installer owns first macOS LaunchAgent enrollment; on Windows and other systems, keep the workspace ready while the native scheduler is added by its platform-specific installer path. Never make normal work wait for background upkeep.
5. Re-read the narrow workspace status after the transaction. Report only a friendly outcome: **pronto**, **pronto com um detalhe para concluir depois** or **preciso de uma escolha sua**. Translate errors; do not show command lines, paths, receipts, internal state names or raw diagnostics.

## Continue naturally

After preparation, offer the next human choice: start the owner interview, create the first account/project, or begin a simple task. Do not make the owner visit folders or run a command to proceed.
