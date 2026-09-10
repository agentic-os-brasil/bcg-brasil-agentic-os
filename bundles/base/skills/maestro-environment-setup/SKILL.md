---
name: maestro-environment-setup
description: Prepare a post-install Maestro workspace conversationally. Use after the Maestro installer has completed, when an owner wants to prepare a workspace, connect Claude or Codex, or make sure the local working environment is ready without following technical setup steps.
---

# Maestro Environment Setup

Prepare one new local workspace and its normal working environment after Maestro has been installed. Keep the mechanics behind Maestro: the owner receives one clear confirmation and a short outcome, not a terminal checklist. For first installation, update, rollback or installer repair, hand off to `$maestro-setup-update`.

Resolve the canonical [`interaction-profile`](../interaction-profile/SKILL.md) skill before responding. Ajustar o tom e o nível de detalhe da resposta ao perfil do usuário antes de apresentar a preparação. Isso afeta apenas a linguagem e a quantidade de detalhe opcional, nunca a transação de configuração, o limite de responsabilidade ou a confirmação.

## Prepare the environment

1. Verificar o ambiente de execução do Maestro e confirmar que o espaço de trabalho está acessível.
2. Ask one short question: **“Posso preparar este espaço do Maestro agora?”** Explain that this creates or refreshes only the local workspace and its integrations; it does not read prior files, publish anything or change another workspace.
3. After agreement, run the installed `setup apply` route once with the active workspace, runtime and executable. This is the deterministic consolidation: it initializes the workspace if needed, installs the runtime projection and hooks, verifies the local projection and reuses the local setup grant on later checkups.
4. Inspect Darwin's user-level maintenance state without trying to replace it. The visual installer owns first macOS LaunchAgent enrollment; on Windows and other systems, keep the workspace ready while the native scheduler is added by its platform-specific installer path. Never make normal work wait for background upkeep.
5. Re-read the narrow workspace status after the transaction. Report only a friendly outcome: **pronto**, **pronto com um detalhe para concluir depois** or **preciso de uma escolha sua**. Translate errors; do not show command lines, paths, receipts, internal state names or raw diagnostics.

## Interpretador local

As rotinas automáticas do Maestro — memória entre conversas, roteamento de skills, separação entre clientes — leem e escrevem JSON através de um interpretador Python 3. Sem ele, todas ficam inertes, e ficam inertes **em silêncio**: cada hook sai sem erro.

O Maestro procura o interpretador sozinho, sob os três nomes que ele costuma ter (`python3`, `python`, e o launcher `py -3` no Windows). Esta seção trata do caso em que existe um interpretador na máquina que não está sob nenhum desses nomes — por exemplo, instalado em um caminho próprio.

**Instalar um interpretador não está autorizado hoje.** A decisão `PYUV` cobre ambiente Python sob demanda para uma capacidade pedida pelo dono, e diz explicitamente que cada nova capacidade dependente de Python precisa da própria justificativa. As rotinas automáticas são infraestrutura sempre-ligada, não capacidade sob demanda, então estão fora desse escopo. Enquanto não houver decisão que as cubra, não instalar nada — reportar e encaminhar.

1. **Confirmar o estado.** Rodar `bash -c '. "$CLAUDE_PROJECT_DIR/.claude/hooks/lib/python.sh"; maestro_python'`. Se imprimir algo, está tudo certo: seguir em silêncio.

2. **Se não imprimir nada, procurar um interpretador fora do PATH** antes de concluir que falta. Só continuar se encontrar um Python 3 utilizável.

3. **Se encontrar, registrar onde ele está.** Gravar o caminho absoluto, em uma linha e sem mais nada, em `brain/.maestro-python`. É esse registro que faz os hooks o encontrarem. O arquivo é texto puro de propósito — é ele que diz onde está o leitor de JSON, e exigir JSON para lê-lo seria circular.

4. **Verificar de verdade.** Repetir o comando do passo 1. Ele tem de imprimir o caminho registrado. Se não imprimir, o registro não serviu: reportar em uma linha e não afirmar que ficou pronto.

5. **Se não encontrar nenhum**, reportar em uma linha que uma peça não está instalada nesta máquina, que o Maestro segue utilizável para conversar, e orientar a avisar o time BCG Brasil AI. Nunca pedir ao dono para instalar Python, abrir terminal ou rodar comando.

## Preparação do componente de leitura avançada de documentos

Tratar o suporte local a documentos como um componente opcional, não como pré-requisito para começar a trabalhar.

1. Verificar se a versão atual do Maestro inclui o componente de leitura avançada de documentos (verified MarkItDown runtime pack).
2. Se não incluir, manter a configuração como completa e informar apenas: **”Seu espaço já está pronto; a leitura avançada de documentos poderá ser adicionada quando o componente aprovado estiver disponível.”** Não tratar como erro nem transformar em chamado de suporte.
3. Do not install MarkItDown from `pip` ou qualquer fonte não gerenciada. Quando a versão aprovada do pacote estiver disponível, o próprio Maestro irá orientar o processo.

## Continue naturally

After preparation, offer the next human choice: start the owner interview, create the first account/project, or begin a simple task. Do not make the owner visit folders or run a command to proceed.
