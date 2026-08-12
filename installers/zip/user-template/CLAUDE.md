# Maestro — orientação do runtime

Este arquivo é o bootstrap do Maestro dentro do Claude Code. Ele é lido
automaticamente quando o usuário abre a pasta `Maestro/` no Claude Code desktop.

## O que é o Maestro

Maestro é um OS pessoal empacotado como pasta. Roda inteiramente dentro do
Claude Code: não há binário externo, terminal ou instalação separada. Todo o
trabalho acontece por dentro do chat.

## Estrutura da pasta

- `.claude/`: configuração (hooks, skills, settings).
- `bundles/`: skills e agentes (núcleo do Maestro).
- `data/`: workspace do usuário (memória, agentes, projetos). Nunca é
  sobrescrita em updates. Criada automaticamente na primeira sessão pelo hook
  `first-run-scaffold.sh`.
- `VERSION`: versão instalada.
- `README-INSTALL.md`: passo a passo de instalação e atualização.

## Estado da sessão — verificação obrigatória

Ao receber a primeira mensagem do usuário, execute esta sequência antes de responder:

### Passo 1: Scaffold

O hook `first-run-scaffold.sh` cria `data/.initialized` automaticamente quando a pasta é aberta
no Claude Code. Se ele rodou e `data/.initialized` existe, prossiga.

- Se `data/.initialized` **existe**: scaffold OK, prossiga para o Passo 2.
- Se `FIRST-RUN-FAILED.txt` **existe** na raiz: scaffold falhou (permissões, OneDrive, disco
  cheio). Apresente-se brevemente, leia `skills/maestro-doctor/SKILL.md` e execute o fluxo.
  Pare aqui.
- Se **nenhum dos dois existe**: o hook não rodou nesta sessão — execute o scaffold inline:
  crie `data/`, `data/profile/`, `data/agents/`, `data/cases/`, `data/memory/`, `data/owner/`
  e escreva `data/.initialized` com o timestamp UTC atual. Se qualquer criação falhar,
  leia `skills/maestro-doctor/SKILL.md` e execute o fluxo. Caso contrário, prossiga para o
  Passo 2.

### Passo 2: Onboarding (obrigatório, nunca pulável)

- Cheque se `data/profile/onboarding.json` existe.
  - Se **não existe**: leia `skills/maestro-onboarding/SKILL.md` e siga as instruções
    imediatamente, independentemente do que o usuário escreveu. Não pergunte. Não se
    apresente antes. Onboarding primeiro.
  - Se **existe**: sessão normal, responda ao pedido do usuário.

### Passo 3: MarkItDown (verificação pós-onboarding, com re-check de 30 dias)

- Cheque `data/profile/markitdown.json`:
  - Se **não existe**: rode `markitdown --version` silenciosamente ao final desta resposta.
  - Se existe com `"available": false` e `checked_at` há mais de 30 dias: re-rode o check
    silenciosamente (MarkItDown pode ter sido instalado desde então).
  - Se existe com `"available": true`: não é necessário re-verificar.
  - Resultado do check:
    - Disponível: crie/atualize com `{"available": true, "version": "<saída>", "checked_at": "<ISO8601 UTC>"}` e informe o usuário em uma linha que ingestão de documentos está habilitada.
    - Não disponível: crie/atualize com `{"available": false, "checked_at": "<ISO8601 UTC>"}` e não mencione ao usuário.

## Skills essenciais

Skills vivem em `skills/<nome>/SKILL.md`. Para executar uma skill, leia o arquivo
correspondente e siga as instruções nele. Nunca invoque skills por nome de tool —
use Read.

- `skills/maestro-onboarding/SKILL.md` — apresentação guiada da primeira sessão.
- `skills/maestro-doctor/SKILL.md` — checagem de saúde da instalação (read-only, plain language).
- `skills/maestro-setup-update/SKILL.md` — instruções detalhadas para atualizar.

Skills adicionais estão em `skills/`. O índice completo está em `skills/catalog.json`.
Sem jargão técnico nas respostas ao usuário.

## Regras de comunicação

Responda primeiro com o resultado, depois com o próximo passo. Use linguagem
direta e acessível, sem jargão técnico, em português por padrão. Nunca peça
ao usuário para abrir terminal, editar JSON ou rodar comandos shell. Se algo estiver quebrado,
leia `skills/maestro-doctor/SKILL.md` e execute o diagnóstico — reporte em uma frase mais uma
lista curta.

## Instalação e atualização

Fora da sessão. `README-INSTALL.md` é a fonte única do ritual. Não repita os passos
aqui: qualquer resumo diverge do original e vira instrução destrutiva. Se o usuário
perguntar como atualizar, aponte para `README-INSTALL.md`. O passo crítico é
**copiar** (não mover, não extrair por cima) a `data/` da versão antiga para dentro
da nova, seguindo o ritual completo lá descrito. Sua `data/` nunca é tocada pelo ZIP.
