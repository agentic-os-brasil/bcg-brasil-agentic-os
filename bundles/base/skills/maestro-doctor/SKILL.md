---
name: maestro-doctor
description: Runs a plain-language health check of the user's Maestro install. Verifies core files, workspace layout, hook wiring and current version. Use whenever the user asks "está tudo OK?", "checar instalação", "diagnosticar Maestro" or similar.
---

# Maestro Doctor

Diagnose a Maestro install without technical jargon. Report in one paragraph plus a short list. Never ask the user to open a terminal — inspect files yourself.

## Execution mode

Determine the mode before running any other check:

- **Repo/Worktree mode** — the opened Git root contains either
  `.bcgos/workspace-projections/claude.json` or
  `.bcgos/workspace-projections/codex.json`. The opened directory is a thin,
  enrolled projection. Do not expect `VERSION`, `bundles/` or `data/` inside
  the repository and do not inspect paths outside the opened worktree directly.
- **Hub mode** — neither projection manifest exists. Use the original ZIP-root checks below.

In Repo/Worktree mode, diagnose the runtime whose skill root loaded this exact
`SKILL.md`; do not diagnose an enrolled sibling runtime unless the user asks.
Use the opened Git root directly rather than a runtime-specific environment
variable. Read only that runtime's local projection manifest and native config.
From the manifest, obtain the opaque workspace ID and exact installed executable
path. Confirm that every binding uses that executable and the exact opened
worktree. Then run that executable with `version`, and its read-only status
command for the selected runtime. A healthy direct projection returns
`enrolled`:

- **Claude direct:** read `.bcgos/workspace-projections/claude.json` and
  `.claude/settings.local.json`; confirm all seven Claude events
  (`SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `Stop`,
  `SubagentStart`, `SubagentStop`); run `workspace status --runtime claude`;
  and confirm the five managed files under `.claude/agents/`. The currently
  loaded Doctor file already proves that its projected skill pointer resolves.
- **Codex direct:** read `.bcgos/workspace-projections/codex.json` and
  `.codex/hooks.json`; confirm the five Codex events (`SessionStart`,
  `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `Stop`); run
  `workspace status --runtime codex`; and confirm
  `.codex/skills/maestro-doctor/SKILL.md`. Codex native hook trust is a separate
  host state: if the current session reports unreviewed hooks, return one point
  to verify and direct the user to review the exact project hooks through
  `/hooks`; never bypass trust in an ordinary Doctor run.

Do not reconstruct or edit commands, do not use a PATH lookup, and do not
follow private paths from the manifest for direct file inspection. These checks
replace checks 1–11 below in Repo/Worktree mode.

## Interaction profile

Resolve `interaction-profile` if present. Adjust vocabulary and depth, never the checks themselves.

## Hub checks (run in order, silent on success)

1. **Core files present** — verify these exist at `${CLAUDE_PROJECT_DIR}`:
   - `VERSION`
   - `CLAUDE.md`
   - `.claude/settings.json`
   - `.claude/hooks/first-run-scaffold.sh`
   - `bundles/base/`, `bundles/tech-core/`

2. **Workspace present and healthy** — verify:
   - `data/` exists and is a directory
   - `data/agents/`, `data/memory/`, `data/profile/`, `data/workspaces/`, `data/owner/` exist
   - `data/.initialized` exists (created by first-run-scaffold on session 1)

3. **Hooks wired** — read `.claude/settings.json` and confirm SessionStart lists both `first-run-scaffold.sh` and `session-start-memory-inject.sh`, and Stop lists `session-stop-dream.sh`.

4. **Version readable** — read `VERSION`, confirm it matches `X.Y.Z` shape.

5. **Hook executable** (Mac/Linux only) — verify the hook under `.claude/hooks/` has execute permission. On Windows the permission bit is not the constraint, so skip it here — whether the hooks ran at all is covered by check 11.

6. **Cloud-sync path** — inspect `${CLAUDE_PROJECT_DIR}` for substrings `OneDrive`, `Dropbox`, `Google Drive`, `iCloud`, `iCloudDrive`, `pCloud`, `Box Sync`. If any match, sinalizar como ponto a verificar. Pasta sincronizada em nuvem pode causar conflitos de arquivo e perda de `data/` durante extração do ZIP novo. Recomendar mover a pasta `Maestro/` para um local não-sincronizado, por exemplo `Documents/Maestro/` (Mac) ou `Documentos\Maestro\` local (Windows).

7. **Problemas conhecidos** — ler `${CLAUDE_PROJECT_DIR}/bundles/base/known-issues.md`. Se o arquivo listar entradas ativas (qualquer coisa além do bloco "nenhuma"), surfar cada uma como um "ponto a verificar" com o contorno indicado. Se listar "nenhuma" ou estiver ausente, seguir em silêncio.

8. **Workspace recuperada** — listar `${CLAUDE_PROJECT_DIR}/data/.recovered-*`. Se houver algum, surfar como ponto informativo (não é erro): "workspace foi recuperada em <timestamp extraído do nome do arquivo>, nenhuma ação necessária". Isso sinaliza que o hook de scaffold detectou `data/` sem marcador (restauração de backup ou marker apagado em update).

9. **Log do scaffold** — se `${CLAUDE_PROJECT_DIR}/data/.scaffold.log` existir, ler as últimas 20 linhas. Se contiver `MKDIR FAIL` ou `ABORT`, surfar como ponto a verificar com a recomendação: "reinstale seguindo o passo a passo do `README-INSTALL.md` na raiz da pasta Maestro. Sua `data/` é preservada porque o ritual copia ela para a instalação nova." Se só houver linhas `MKDIR OK` e `DONE`, seguir em silêncio.

10. **Drift de árvore do owner (onboarding pré-fix)** — detectar owners que fizeram onboarding antes do fix de fechamento da árvore de controle. Ler:
    - `data/owner/registry.json` → campo `initialized`
    - `data/owner/interview/confirmations.json` → campo `completed_tracks`
    - `data/profile/onboarding.json` → campo `status`

    Se `onboarding.json.status == "complete"` **e** (`registry.json.initialized == false` **ou** `confirmations.json.completed_tracks == []`), surfar como ponto informativo (não é erro funcional): "o onboarding foi concluído numa versão anterior do Maestro e dois arquivos de controle interno ficaram desatualizados. Nenhuma ação necessária — rodar o onboarding uma vez de novo (opcional) atualiza os arquivos. Isso não afeta o funcionamento do sistema." Se ambos os campos já refletem o estado concluído, seguir em silêncio.

11. **As rotinas automáticas rodaram nesta máquina** — este é o único check que
    detecta uma instalação onde os hooks nunca executaram. Comparar dois arquivos:

    - `data/.initialized` existe (a workspace foi montada), **e**
    - `data/.scaffold.log` **não** existe.

    `first-run-scaffold.sh` escreve o log em toda execução, desde a primeira linha.
    O caminho de emergência descrito no `CLAUDE.md` — em que o próprio assistente
    monta a `data/` dentro da conversa — não escreve o log. Portanto `data/`
    montada **sem** log significa que os hooks não rodaram: em geral porque o
    `bash` não está disponível na máquina (ele não vem no Windows por padrão).

    Quando o par acima bater, surfar como ponto a verificar, em linguagem simples:
    "algumas rotinas automáticas do Maestro não estão ativas nesta máquina — ele
    funciona, mas não lembra sozinho do contexto entre conversas nem fecha o dia
    por conta própria. Avise o time BCG Brasil AI; não é problema da sua pasta e
    não dá pra resolver por aqui." Complementar com o contorno da entrada
    `hooks-nao-executados` em `known-issues.md`.

    Se `data/.scaffold.log` existir, ou se `data/.initialized` não existir (aí o
    caso é o check 2, não este), seguir em silêncio.

    Nunca recomendar instalar `bash`, Git for Windows ou WSL ao usuário: é
    decisão de quem administra a máquina e foge do contrato de "nada de terminal".

## Output shape

Return a single message with:

- **One-line verdict:** "Tudo funcionando" | "Um ponto a verificar: <what>" | "Instalação incompleta: <what>"
- **Version:** `v<X.Y.Z>` (from `VERSION` in Hub mode or the exact installed executable in Repo/Worktree mode)
- **Sua workspace:** absolute path to `data/` in Hub mode; in Repo/Worktree mode, the opened repository path plus its opaque workspace ID (never expose the private `data_root`)
- **Se houver problemas:** action per problem, in plain Portuguese. Para arquivos core ausentes, apontar para o `README-INSTALL.md` na raiz da pasta Maestro — ele é a fonte única do ritual de instalação e atualização. Nunca orientar a extrair o ZIP por cima da pasta atual: isso mistura arquivos de versões diferentes. Nunca repetir os passos do ritual aqui; qualquer resumo diverge do original. Nunca pedir para o usuário editar JSON ou shell.

## What NOT to do

- In Hub mode, do not run `bcgos`; the Hub diagnosis is file-based. In Repo/Worktree mode, run only the exact absolute executable recorded by the projection, and only its read-only `version` and `workspace status` commands.
- Do not try to install, update or repair anything. `maestro-doctor` is read-only.
- Do not dump raw JSON, file contents, hashes ou timestamps salvo se o usuário pedir.
- Do not surface intermediate check names; report the outcome, not the procedure.

## Escalation

If more than 2 core files are missing, tell the user: "sua instalação parece corrompida — baixe o ZIP mais recente pelo email do time BCG Brasil AI e siga o passo a passo do `README-INSTALL.md`. Sua `data/` é preservada pelo ritual."

Se `data/` estiver ausente mas core existir, apenas informe: "sua próxima sessão vai recriar `data/` automaticamente."
