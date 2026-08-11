---
name: maestro-environment-setup
description: Prepare a post-install Maestro workspace conversationally. Use after the Maestro installer has completed, when an owner wants to prepare a workspace, connect Claude or Codex, or make sure the local working environment is ready without following technical setup steps.
---

# Maestro Environment Setup

Prepare one new local workspace and its normal working environment after Maestro has been installed. Keep the mechanics behind Maestro: the owner receives one clear confirmation and a short outcome, not a terminal checklist. For first installation, update, rollback or installer repair, hand off to `$maestro-setup-update`.

Resolve `interaction-profile` before presenting the preparation. It changes only
the language and amount of optional detail, never the setup transaction,
ownership boundary or confirmation.

## Prepare the environment

1. Resolve the active runtime and the exact installed Maestro CLI path from SessionStart. Do not rely on `PATH`, substitute a source checkout or ask the owner to find an executable.
2. Ask one short question: **“Posso preparar este espaço do Maestro agora?”** Explain that this creates or refreshes only the local workspace and its integrations; it does not read prior files, publish anything or change another workspace.
3. After agreement, run the installed `setup apply` route once with the active workspace, runtime and executable. This is the deterministic consolidation: it initializes the workspace if needed, installs the runtime projection and hooks, verifies the local projection and reuses the local setup grant on later checkups.
4. Inspect Darwin's user-level maintenance state without trying to replace it. The visual installer owns first macOS LaunchAgent enrollment; on Windows and other systems, keep the workspace ready while the native scheduler is added by its platform-specific installer path. Never make normal work wait for background upkeep.
5. Re-read the narrow workspace status after the transaction. Report only a friendly outcome: **pronto**, **pronto com um detalhe para concluir depois** or **preciso de uma escolha sua**. Translate errors; do not show command lines, paths, receipts, internal state names or raw diagnostics.

## MarkItDown preparation

Treat local document support as an optional prepared component, not a prerequisite for beginning work.

1. Inspect whether the current managed release includes a verified MarkItDown runtime pack.
2. If it does not, keep setup complete and say only: **“Seu espaço já está pronto; a leitura avançada de documentos poderá ser adicionada quando o componente aprovado estiver disponível.”** Do not call it an error or turn it into a support task.
3. Do not install MarkItDown from `pip`, a package manager, a browser download or an ambient Python environment. When a verified managed pack ships, route the owner through its versioned product flow.

## Continue naturally

After preparation, offer the next human choice: start the owner interview, create the first account/project, or begin a simple task. Do not make the owner visit folders or run a command to proceed.
