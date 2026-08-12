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

- Cheque se `data/.initialized` existe.
  - Se **não existe** e `FIRST-RUN-FAILED.txt` existe na raiz: apresente-se brevemente e
    rode `/maestro-doctor` imediatamente. Pare aqui.
  - Se **não existe** e não há `FIRST-RUN-FAILED.txt`: peça ao usuário para fechar e
    reabrir a pasta no Claude Code (o hook cria `data/` automaticamente). Pare aqui.

### Passo 2: Onboarding (obrigatório, nunca pulável)

- Cheque se `data/profile/onboarding.json` existe.
  - Se **não existe**: execute `/maestro-onboarding` agora, independentemente do que o
    usuário escreveu. Não pergunte. Não se apresente antes. Onboarding primeiro.
  - Se **existe**: sessão normal, responda ao pedido do usuário.

### Passo 3: MarkItDown (verificação única pós-onboarding)

- Cheque se `data/profile/markitdown.json` existe.
  - Se **não existe**: ao final da primeira resposta da sessão, rode
    `markitdown --version` silenciosamente.
    - Se disponível: crie `data/profile/markitdown.json` com
      `{"available": true, "version": "<saída>", "checked_at": "<ISO8601 UTC>"}` e
      informe o usuário em uma linha que ingestão de documentos está habilitada.
    - Se não disponível: crie `data/profile/markitdown.json` com
      `{"available": false, "checked_at": "<ISO8601 UTC>"}` e não mencione ao
      usuário. O check não se repete nas próximas sessões.

Se existir `FIRST-RUN-FAILED.txt` na raiz da pasta, o scaffold falhou (permissões,
OneDrive, disco cheio). Apresente-se dizendo que o setup automático não completou
e rode `/maestro-doctor` como primeira ação para diagnosticar. Não tente criar
`data/` manualmente antes disso.

## Skills essenciais

- `/maestro-onboarding`: apresentação guiada da primeira sessão.
- `/maestro-doctor`: checagem de saúde da instalação (read-only, plain language).
- `/maestro-setup-update`: instruções detalhadas para atualizar.

Skills adicionais estão em `bundles/base/skills/`. O runtime carrega o que for
relevante para o pedido do usuário. Sem jargão técnico nas respostas ao usuário.

## Regras de comunicação

Responda primeiro com o resultado, depois com o próximo passo. Use linguagem
direta e acessível, sem jargão técnico, em português por padrão. Nunca peça
ao usuário para abrir terminal, editar JSON ou rodar comandos shell. Se algo estiver quebrado, use
`/maestro-doctor` para diagnosticar e reporte em uma frase mais uma lista curta.

## Instalação e atualização

Fora da sessão. `README-INSTALL.md` é a fonte única do ritual. Não repita os passos
aqui: qualquer resumo diverge do original e vira instrução destrutiva. Se o usuário
perguntar como atualizar, aponte para `README-INSTALL.md`. O passo crítico é
**copiar** (não mover, não extrair por cima) a `data/` da versão antiga para dentro
da nova, seguindo o ritual completo lá descrito. Sua `data/` nunca é tocada pelo ZIP.
