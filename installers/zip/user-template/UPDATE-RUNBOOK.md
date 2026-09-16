---
contract_id: maestro-update-long-run-v1
schema_version: 1
model_family: opus
minimum_effort: high
preferred_effort: xhigh
permission_mode: auto-when-available-and-user-selected
receipt_statuses:
  - in_progress
  - pass
  - fail
  - unavailable
check_states:
  - PASS
  - FAIL
  - UNAVAILABLE
required_subagents:
  - yoda
progress_receipt: data/canary/update-<to_version>.json
receipt_bindings:
  - attempt_id
  - from_version
  - to_version
  - platform
  - target_release_sha256
  - target_core_sha256
  - baseline_manifest_sha256
  - installation_root_sha256
---

# Verificação longa de uma atualização

Este arquivo é o contrato de execução da verificação, não um instalador.
O front matter acima é declarativo e deve ser lido pelo Maestro; ele não
altera sozinho o modelo, o modo de permissão ou a política corporativa.
O diretório histórico `data/canary/` é mantido somente como namespace
compatível de receipts; o resultado produzido por este contrato é uma
qualificação de release válida para o ZIP exato, não um canário preliminar.

## Preparar a sessão

Abra o Claude Code a partir da raiz da nova pasta `Maestro` e aceite a
confiança somente para esse caminho. Antes de iniciar a verificação:

1. Em `/model`, escolha `opus`. O alias usa o Opus mais novo permitido pela
   organização; confirme no cabeçalho qual versão foi realmente selecionada.
   Se o ambiente BCG disponibilizar Opus 4.7, ele é compatível com este fluxo.
2. Execute `/effort xhigh`. Se a política limitar esse nível, use o maior
   disponível, nunca abaixo de `high`. `/effort auto` volta ao padrão do modelo;
   confirme o nível efetivo no cabeçalho antes de continuar.
3. Se o seletor de permissão mostrar **Auto mode**, ele pode ser escolhido para
   reduzir interrupções. Auto é um modo de permissão com verificação de
   segurança, não um modelo e não uma autorização para contornar políticas. Se
   estiver indisponível ou bloqueado pela organização, siga com o modo normal.
4. Ative a continuidade colando **todo** o conteúdo de
   `PROMPT-2-VERIFICAR.txt`. A primeira linha começa com `/goal`, e o próprio
   goal manda ler este runbook e a skill, retomar o receipt e executar a
   verificação. O `/goal` inicia a execução imediatamente; não espere uma
   etapa separada depois dele.
5. Se `/goal` estiver indisponível por versão ou política, execute
   `/maestro-setup-update`, escolha **atualização** e registre no receipt que a
   continuidade por goal ficou `UNAVAILABLE`.

O `/goal` depende de uma workspace confiável e de hooks permitidos. Se a
política corporativa o bloquear, não tente contornar a política. A verificação
pode continuar com checkpoints, mas essa limitação deve aparecer como
`UNAVAILABLE` no receipt.

## Contrato do Maestro

Depois de iniciado, o Maestro deve:

1. criar ou retomar `data/canary/update-<to_version>.json` sem registrar
   conteúdo pessoal, prompts ou material de cliente. Antes de reutilizar
   qualquer `PASS`, validar todas as bindings do front matter: UUID opaco da
   tentativa, versões, plataforma, SHA-256 do ZIP exato conferido contra o
   sidecar, SHA-256 agregado de todos os arquivos do core fora de `data/`,
   SHA-256 do manifesto de baseline e SHA-256 do caminho canônico da raiz. Uma
   binding ausente ou divergente invalida a tentativa anterior;
2. preservar um receipt incompatível com sufixo
   `-stale-<attempt_id>.json`, iniciar outro `attempt_id` e trabalhar a partir
   do primeiro check pendente da tentativa atual, sem reiniciar seus checks
   verdes;
3. verificar versão, baseline SHA-256, preservação de `data/`, runtime, hooks
   carregados e executados, e projeções de agentes. `/status` e `/hooks` são a
   rota preferida quando existirem; em versões que não os exponham, aceitar
   somente um traço `stream-json` com `init` na raiz nova e respostas bem
   sucedidas dos hooks SessionStart, UserPromptSubmit, PreToolUse/Agent e Stop,
   combinado com a leitura dos seis handlers em `.claude/settings.json`;
4. corrigir ou diagnosticar achados reversíveis dentro do escopo autorizado;
5. despachar o subagent `yoda` com o pacote final de evidência para pressionar
   a conclusão;
6. continuar enquanto houver trabalho seguro e acionável;
7. encerrar somente com todos os checks em `PASS`, `FAIL` ou `UNAVAILABLE` e o
   receipt final persistido. O campo raiz `status` usa exclusivamente
   `in_progress`, `pass`, `fail` ou `unavailable` em minúsculas; cada
   `checks[*].state` e cada `agents.<id>.state` usa `PASS`, `FAIL` ou
   `UNAVAILABLE` em maiúsculas.

Uma chamada de subagent só conta quando a ferramenta `Agent` foi usada e o
retorno chegou à sessão principal. Ler o arquivo do agente ou redigir uma
resposta em seu nome não conta. `UNAVAILABLE` é o resultado correto quando
modelo, Agent, hook, permissão ou política impedem a prova; nunca fabrique um
`PASS` para encerrar.

Se houver interrupção, limite de uso, erro de API ou compactação, reabra a
mesma pasta e cole `PROMPT-2-VERIFICAR.txt` novamente; se `/goal` não existir,
execute `/maestro-setup-update`. O receipt determina o próximo passo. Não
apague `Maestro-old/` até o campo raiz ser exatamente `status: pass`.
