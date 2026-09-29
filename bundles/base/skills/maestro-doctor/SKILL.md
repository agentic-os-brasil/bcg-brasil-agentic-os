---
name: maestro-doctor
description: Runs a plain-language health check of the user's Maestro install. Verifies core files, workspace layout, hook wiring and current version. Use whenever the user asks "está tudo OK?", "checar instalação", "diagnosticar Maestro" or similar.
---

# Maestro Doctor

Diagnose a Maestro install without technical jargon. Report in one paragraph plus a short list. Never ask the user to open a terminal — inspect files yourself.

## Interaction profile

Resolve [`interaction-profile`](../interaction-profile/SKILL.md) if present. Adjust vocabulary and depth, never the checks themselves.

## Checks (run in order, silent on success)

0. **Migração validada agora** — antes de considerar `.initialized`, se `data/`
   ou `brain/.maestro/migration/state.json` existir, executar somente leitura pelo
   helper gerenciado e verificado: `maestro-runtime migration --project <raiz> --status`.
   No Mac, usar `.claude/hooks/lib/maestro-runtime.sh` e
   `maestro_runtime <raiz> migration --project <raiz> --status`; no Windows,
   usar o adaptador PowerShell do mesmo helper. Não executar binário sem validar
   manifesto, versão e SHA256. Ausência do helper é `unavailable`, nunca sucesso.
   O comando revalida inventário e hashes da origem, plano imutável e recibo da
   mesma tentativa, além dos destinos. Ler um JSON antigo não substitui isso.

   - `committed` ou `not_needed`: continuar os demais checks. `committed` prova
     esta cópia validada, não qualificação do runtime ou autorização para apagar backup.
Se verification=target_evolved e current=false, o recibo comprova a cópia
     histórica e o dono já alterou brain/. Isso não bloqueia uso normal, não
     autoriza recopiar arquivos e não pode ser reutilizado como PASS atual de
     atualização. Reportar como informativo, mantendo a instalação utilizável.
   - `planned`, `blocked` ou `partial`: diagnóstico de atualização incompleta;
     mostrar que a origem está preservada, o impedimento concreto e o próximo
     passo via `/maestro-setup-update`. Não criar marcadores, não inicializar uma
     pasta alternativa, não recomendar remover `data/` ou `Maestro-old`.
   - `rolled_back`: a tentativa foi revogada; `restored_runtime=false` significa
     que nenhum runtime anterior foi restaurado. Manter atualização em HOLD até
     reabrir e validar a instalação anterior preservada. Conteúdo novo em `brain/`
     também permanece intacto.

   Para conteúdo legado em `agents`, `workspaces` ou `canary`, usar
   `migration --project <raiz> --resolve-legacy <namespace>` e ler apenas o caminho
   retornado após `committed`. Esses diretórios continuam na origem por adaptador
   explícito; não inventar um destino, presumir que estavam vazios ou copiar
   dados para outro cliente. O adaptador é de leitura e falha se os hashes mudarem.
   Não recomendar descarte de `data/` enquanto houver conteúdo retido no mapa.

1. **Core files present** — verify these exist at `${CLAUDE_PROJECT_DIR}`:
   - `VERSION`
   - `CLAUDE.md`
   - `.claude/settings.json`
   - o hook de scaffold indicado por `.claude/settings.json` (`.ps1` no
     Windows, `.sh` no Mac)
   - `bundles/base/`, `bundles/tech-core/`

2. **Workspace present and healthy** — verify:
   - `brain/` exists and is a directory
   - the nine top-level trees exist: `brain/memory/`, `brain/owner/`, `brain/daily/`,
     `brain/learnings/`, `brain/craft/`, `brain/people/`, `brain/development/`,
     `brain/accounts/`, `brain/tasks/`
   - `brain/.initialized` exists (created by first-run-scaffold on session 1)

3. **Hooks wired** — ler `.claude/settings.json`, identificar o perfil pelo
   sufixo dos comandos e confirmar SessionStart com scaffold e memória, Stop
   com dream, UserPromptSubmit com injeção de contexto e PreToolUse com proteção
   de escrita e anúncio de Agent/Task. Perfil Windows válido: todos os seis
   comandos terminam em `.ps1`, cada handler declara `"shell": "powershell"`
   e nenhum comando chama Bash. Perfil Mac válido: todos os seis comandos
   terminam em `.sh`.

4. **Version readable** — read `VERSION`, confirm it matches `X.Y.Z` shape.

5. **Hook executable** (Mac/Linux only) — verify the hook under `.claude/hooks/` has execute permission. On Windows the permission bit is not the constraint, so skip it here — whether the hooks ran at all is covered by check 11.

6. **Cloud-sync path** — inspect `${CLAUDE_PROJECT_DIR}` for substrings `OneDrive`, `Dropbox`, `Google Drive`, `iCloud`, `iCloudDrive`, `pCloud`, `Box Sync`. If any match, sinalizar como ponto a verificar. Pasta sincronizada em nuvem pode causar conflitos de arquivo e perda de `brain/` durante extração do ZIP novo. Recomendar mover a pasta `Maestro/` para um local não-sincronizado, por exemplo `Documents/Maestro/` (Mac) ou `Documentos\Maestro\` local (Windows).

7. **Problemas conhecidos** — ler `${CLAUDE_PROJECT_DIR}/bundles/base/known-issues.md`. Se o arquivo listar entradas ativas (qualquer coisa além do bloco "nenhuma"), surfar cada uma como um "ponto a verificar" com o contorno indicado. Se listar "nenhuma" ou estiver ausente, seguir em silêncio.

8. **Workspace recuperada** — listar `${CLAUDE_PROJECT_DIR}/brain/.recovered-*`. Se houver algum, surfar como ponto informativo (não é erro): "workspace foi recuperada em <timestamp extraído do nome do arquivo>, nenhuma ação necessária". Isso sinaliza que o hook de scaffold detectou `brain/` sem marcador (restauração de backup ou marker apagado em update).

9. **Log do scaffold** — se `${CLAUDE_PROJECT_DIR}/brain/.scaffold.log` existir, ler as últimas 20 linhas. Se contiver `MKDIR FAIL` ou `ABORT`, surfar como ponto a verificar com a recomendação: "reinstale seguindo o passo a passo do `README-INSTALL.md` na raiz da pasta Maestro. Sua `brain/` é preservada porque o ritual copia ela para a instalação nova." Se só houver linhas `MKDIR OK` e `DONE`, seguir em silêncio.

10. **Drift de árvore do owner (onboarding pré-fix)** — detectar owners que fizeram onboarding antes do fix de fechamento da árvore de controle. Ler:
    - `brain/owner/registry.json` → campo `initialized`
    - `brain/owner/interview/confirmations.json` → campo `completed_tracks`
    - `brain/owner/onboarding.json` → campo `status`

    Se `onboarding.json.status == "complete"` **e** (`registry.json.initialized == false` **ou** `confirmations.json.completed_tracks == []`), surfar como ponto informativo (não é erro funcional): "o onboarding foi concluído numa versão anterior do Maestro e dois arquivos de controle interno ficaram desatualizados. Nenhuma ação necessária — rodar o onboarding uma vez de novo (opcional) atualiza os arquivos. Isso não afeta o funcionamento do sistema." Se ambos os campos já refletem o estado concluído, seguir em silêncio.

11. **As rotinas automáticas rodaram nesta máquina** — este é o único check que
    detecta uma instalação onde os hooks nunca executaram. Comparar dois arquivos:

    - `brain/.initialized` existe (a workspace foi montada), **e**
    - `brain/.scaffold.log` **não** existe.

    O hook `first-run-scaffold` escreve o log em toda execução, desde a primeira linha.
    O caminho de emergência descrito no `CLAUDE.md` — em que o próprio assistente
    monta a `brain/` dentro da conversa — não escreve o log. Portanto `brain/`
    montada **sem** log significa que os hooks não rodaram ou não foram aceitos
    pelo Claude Code.

    Quando o par acima bater, surfar como ponto a verificar, em linguagem simples:
    "algumas rotinas automáticas do Maestro não estão ativas nesta máquina — ele
    funciona, mas não lembra sozinho do contexto entre conversas nem fecha o dia
    por conta própria. Avise o time BCG Brasil AI; não é problema da sua pasta e
    não dá pra resolver por aqui." Complementar com o contorno da entrada
    `README-INSTALL.md`.

    Se `brain/.scaffold.log` existir, ou se `brain/.initialized` não existir (aí o
    caso é o check 2, não este), seguir em silêncio.

    Nunca recomendar instalar Bash, Git for Windows ou WSL. No perfil Windows
    desta versão, essas peças não participam dos hooks.

12. **Interpretador das rotinas automáticas** — o check 11 responde "os hooks
    rodaram?"; este responde "eles tinham com que trabalhar?". Primeiro ler
    `.claude/settings.json` para identificar o perfil. No perfil Windows
    PowerShell, confirmar PowerShell pela execução já comprovada no check 11 e
    seguir em silêncio: os hooks `.ps1` não dependem de Git Bash nem Python.

    Somente no perfil Mac, todo hook que lê ou escreve JSON depende de um
    interpretador Python 3. Sem ele, a memória em
    Markdown ainda é injetada, mas contexto estruturado e roteamento automático
    de skills e agentes ficam indisponíveis. A separação entre clientes não fica
    desligada: o guard recusa escritas em arquivo que não consegue verificar.

    Rodar `bash -c '. "$CLAUDE_PROJECT_DIR/.claude/hooks/lib/python.sh"; maestro_python'`.

    - Se imprimir algo, seguir em silêncio. O que imprime é o nome ou caminho
      resolvido; não mostrar ao usuário.
    - Se não imprimir nada, surfar como ponto a verificar: "uma peça do Maestro
      não está instalada nesta máquina. A memória em texto continua disponível,
      mas o roteamento automático fica desligado e escritas que exigem validação
      de cliente são recusadas. Ele segue utilizável para conversar. Avise o time
      BCG Brasil AI." Encaminhar para a seção "Interpretador local" de
      `maestro-environment-setup`, que sabe
      apontar o Maestro para um interpretador que exista fora do PATH.

    Instalar um interpretador não está autorizado hoje: a decisão `PYUV` cobre
    ambiente Python sob demanda para uma capacidade pedida pelo dono, e exige
    justificativa própria para cada nova capacidade dependente de Python. As
    rotinas automáticas são infraestrutura sempre-ligada, não capacidade sob
    demanda. Enquanto isso não for decidido, este check reporta e encaminha —
    nunca pede ao usuário para instalar Python nem abrir terminal.

    `maestro-doctor` continua read-only: este check diagnostica e nomeia o
    próximo passo; quem executa é a skill de setup.

12. **Interpretador das rotinas automáticas** — o check 11 responde "os hooks
    rodaram?"; este responde "eles tinham com que trabalhar?". Todo hook que lê
    ou escreve JSON depende de um interpretador Python 3, e sem ele cada um sai
    sem erro: memória entre conversas, roteamento de skills e separação entre
    clientes ficam desligados sem nenhum sinal.

    Rodar `bash -c '. "$CLAUDE_PROJECT_DIR/.claude/hooks/lib/python.sh"; maestro_python'`.

    - Se imprimir algo, seguir em silêncio. O que imprime é o nome ou caminho
      resolvido; não mostrar ao usuário.
    - Se não imprimir nada, surfar como ponto a verificar: "uma peça do Maestro
      não está instalada nesta máquina — por isso ele não lembra do contexto
      entre conversas nem protege a separação entre clientes. Ele segue
      utilizável para conversar. Avise o time BCG Brasil AI." Encaminhar para a
      seção "Interpretador local" de [`maestro-environment-setup`](../maestro-environment-setup/SKILL.md), que sabe
      apontar o Maestro para um interpretador que exista fora do PATH.

    Instalar um interpretador não está autorizado hoje: a decisão `PYUV` cobre
    ambiente Python sob demanda para uma capacidade pedida pelo dono, e exige
    justificativa própria para cada nova capacidade dependente de Python. As
    rotinas automáticas são infraestrutura sempre-ligada, não capacidade sob
    demanda. Enquanto isso não for decidido, este check reporta e encaminha —
    nunca pede ao usuário para instalar Python nem abrir terminal.

    `maestro-doctor` continua read-only: este check diagnostica e nomeia o
    próximo passo; quem executa é a skill de setup.

13. **Sessões agendadas — o que foi combinado ainda existe** — os checks 11 e 12
    tratam dos hooks, que rodam dentro de cada conversa. Este trata das
    *sessões agendadas* de abrir o dia, fechar o dia e retro semanal, que
    rodam sem ninguém: o dono escolheu no onboarding e desde então não tem
    como saber se continuam de pé. Um agendamento que parou é invisível pelo
    mesmo motivo que a rotina é útil — ninguém está lá quando ela deveria
    rodar.

    Ler `brain/owner/operating/scheduled-routines.md`.

    - Ausente, ou `status: declined` → seguir em silêncio. Não foi oferecido
      ainda, ou o dono disse não, e a recusa é sticky: este check não
      reabre o assunto.
    - `status: deferred-tool-unavailable` → surfar como ponto informativo: "as
      rotinas automáticas foram combinadas mas o agendamento não estava
      disponível naquele momento; dá para configurar quando quiser, é só
      pedir." Não é erro.
    - `status: enabled` ou `enabled-partial` → conferir se as tarefas
      registradas ainda existem, listando as tarefas agendadas desta sessão e
      cruzando pelos `taskId` gravados na página. Para cada `taskId` que não
      aparecer, surfar como ponto a verificar, nomeando a rotina em
      linguagem do dono: "o fechamento do dia estava agendado para 23h30 e
      esse agendamento não está mais ativo — dá para recriar, é só pedir."
      Se a listagem de tarefas agendadas não estiver disponível nesta sessão,
      dizer isso em uma linha e não concluir nada: ausência de ferramenta não
      é ausência de agendamento, e afirmar que a rotina caiu quando não se
      pode verificar é pior que não checar.

    Este check nunca cria, recria ou remove uma tarefa agendada. `maestro-doctor`
    é read-only; recriar é ato do dono, na conversa.


## Output shape

Return a single message with:

- **One-line verdict:** "Tudo funcionando" | "Um ponto a verificar: <what>" | "Instalação incompleta: <what>"
- **Version:** `v<X.Y.Z>`
- **Sua workspace:** absolute path to `brain/`
- **Se houver problemas:** action per problem, in plain Portuguese. Para arquivos core ausentes, apontar para o `README-INSTALL.md` na raiz da pasta Maestro — ele é a fonte única do ritual de instalação e atualização. Nunca orientar a extrair o ZIP por cima da pasta atual: isso mistura arquivos de versões diferentes. Nunca repetir os passos do ritual aqui; qualquer resumo diverge do original. Nunca pedir para o usuário editar JSON ou shell.

## What NOT to do

- Do not run `bcgos` (does not exist anymore).
- Do not try to install, update or repair anything. `maestro-doctor` is read-only.
- Do not dump raw JSON, file contents, hashes ou timestamps salvo se o usuário pedir.
- Do not surface intermediate check names; report the outcome, not the procedure.

## Escalation

If more than 2 core files are missing, tell the user: "sua instalação parece corrompida — baixe o ZIP mais recente pelo email do time BCG Brasil AI e siga o passo a passo do `README-INSTALL.md`. Sua `brain/` é preservada pelo ritual."

Se `brain/` estiver ausente mas core existir, apenas informe: "sua próxima sessão vai recriar `brain/` automaticamente."
