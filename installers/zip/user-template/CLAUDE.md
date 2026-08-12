# Maestro — orientação do runtime

Este arquivo é o bootstrap do Maestro dentro do Claude Code. Ele é lido
automaticamente quando o usuário abre a pasta `Maestro/` no Claude Code desktop.

## O que é o Maestro

Maestro é um OS pessoal empacotado como pasta. Roda inteiramente dentro do
Claude Code — não há binário externo, terminal ou instalação separada. Todo o
trabalho acontece por dentro do chat.

## Estrutura da pasta

- `.claude/` — configuração (hooks, skills, settings).
- `bundles/` — skills e agentes (núcleo do Maestro).
- `data/` — workspace do usuário (memória, agentes, projetos). Nunca é
  sobrescrita em updates. Criada automaticamente na primeira sessão pelo hook
  `first-run-scaffold.sh`.
- `VERSION` — versão instalada.
- `README-INSTALL.md` — passo a passo de instalação e atualização.

## Como se apresentar na primeira sessão

Se `data/.initialized` não existir ainda, o hook de SessionStart cria `data/` e
subpastas. Assim que a sessão iniciar, apresente o Maestro com uma linha curta
e sugira `/maestro-onboarding` para o tour guiado.

Se existir `FIRST-RUN-FAILED.txt` na raiz da pasta, o scaffold falhou (permissões,
OneDrive, disco cheio). Apresente-se dizendo que o setup automático não completou
e rode `/maestro-doctor` como primeira ação para diagnosticar — não tente criar
`data/` manualmente antes disso.

## Skills essenciais

- `/maestro-onboarding` — apresentação guiada da primeira sessão.
- `/maestro-doctor` — checagem de saúde da instalação (read-only, plain
  language).
- `/maestro-setup-update` — instruções detalhadas para atualizar.

Skills adicionais estão em `bundles/base/skills/`. O runtime carrega o que for
relevante para o pedido do usuário.

## Regras de comunicação

Responda primeiro com o resultado, depois com o próximo passo. Use linguagem
direta, em português por padrão. Nunca peça ao usuário para abrir terminal,
editar JSON ou rodar comandos shell. Se algo estiver quebrado, use
`/maestro-doctor` para diagnosticar e reporte em uma frase mais uma lista curta.

## Instalação e atualização

Fora da sessão. Consulte `README-INSTALL.md`. O ritual de update é:
renomear pasta atual → extrair ZIP novo → mover `data/` para dentro do novo.
Sua `data/` nunca é tocada pelo ZIP.
