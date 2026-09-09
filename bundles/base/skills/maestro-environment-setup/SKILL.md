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

## Interpretador local

As rotinas automáticas do Maestro — memória entre conversas, roteamento de skills, separação entre clientes — leem e escrevem JSON através de um interpretador Python 3. Sem ele, todas ficam inertes, e ficam inertes **em silêncio**: cada hook sai sem erro. Esta seção existe para que isso nunca dependa do dono saber que precisa instalar algo.

Executar quando o contexto da sessão trouxer `<!-- maestro:python-missing -->`, quando o `maestro-doctor` apontar este ponto, ou quando o dono pedir para preparar o ambiente.

1. **Confirmar que falta mesmo.** Rodar `bash -c '. "$CLAUDE_PROJECT_DIR/.claude/hooks/lib/python.sh"; maestro_python'`. Se imprimir algo, já existe interpretador: não instalar nada, seguir em silêncio. Só continuar quando o comando não imprimir nada.

2. **Pedir confirmação, em uma linha.** "Falta uma peça para o Maestro lembrar do contexto entre conversas e proteger a separação entre clientes. Posso instalar agora? São poucos MB, não precisa de privilégio de administrador e não mexe em mais nada da sua máquina." Se o dono recusar, registrar a recusa em uma linha e seguir; **nunca** insistir na mesma sessão e nunca pedir para ele instalar por conta própria.

3. **Garantir o `uv`.** Verificar com `uv --version`. Se faltar, a decisão `UVIN` autoriza o próprio Maestro a instalá-lo — executar o instalador oficial da astral.sh exatamente como publicado, sem espelhar nem modificar:
   - Mac/Linux: `curl -LsSf https://astral.sh/uv/install.sh | sh`
   - Windows: `powershell -ExecutionPolicy ByPass -c "irm https://astral.sh/uv/install.ps1 | iex"`

4. **Instalar o interpretador.** `uv python install 3.12`, e depois obter o caminho com `uv python find 3.12`.

5. **Registrar onde ele está.** Gravar o caminho absoluto, em uma linha e sem mais nada, em `data/.maestro-python`. Este passo não é opcional: o Python que o `uv` instala vive no diretório dele e não entra no PATH como `python3`, `python` nem `py`, então sem o registro os hooks não o encontram e a instalação inteira não serviu para nada. O arquivo é texto puro de propósito — é ele que diz onde está o leitor de JSON, e exigir JSON para lê-lo seria circular.

6. **Verificar de verdade.** Repetir o comando do passo 1. Ele tem de imprimir o caminho registrado. Se não imprimir, a instalação não pegou: reportar em uma linha e **não** afirmar que ficou pronto.

7. **Reportar em uma linha**, no tom de resultado: "pronto — o Maestro já lembra do contexto entre conversas a partir da próxima sessão." Não mostrar comando, caminho, saída de instalador nem nome de arquivo interno.

**Quando falhar.** Rede indisponível, proxy corporativo bloqueando a astral.sh, ou política da máquina impedindo a execução: reportar em uma linha o que não deu, dizer que o Maestro segue utilizável para conversar, e orientar a avisar o time BCG Brasil AI. Não tentar caminho alternativo, não pedir para o dono baixar nada e não mandar abrir terminal.

## Preparação do componente de leitura avançada de documentos

Tratar o suporte local a documentos como um componente opcional, não como pré-requisito para começar a trabalhar.

1. Verificar se a versão atual do Maestro inclui o componente de leitura avançada de documentos (verified MarkItDown runtime pack).
2. Se não incluir, manter a configuração como completa e informar apenas: **”Seu espaço já está pronto; a leitura avançada de documentos poderá ser adicionada quando o componente aprovado estiver disponível.”** Não tratar como erro nem transformar em chamado de suporte.
3. Do not install MarkItDown from `pip` ou qualquer fonte não gerenciada. Quando a versão aprovada do pacote estiver disponível, o próprio Maestro irá orientar o processo.

## Continue naturally

After preparation, offer the next human choice: start the owner interview, create the first account/project, or begin a simple task. Do not make the owner visit folders or run a command to proceed.
