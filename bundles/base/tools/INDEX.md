# Maestro Tools & Hooks Index

> Índice do que roda por trás do Maestro — fora do `brain/`. Abra o arquivo apontado só quando a tarefa exigir; a maioria destas ferramentas não é chamada diretamente, é lida por um hook ou por uma skill.

## Tools (`bundles/base/tools/`)

| Tool | O que faz | Pointer |
|---|---|---|
| Roteador de agentes | Decide qual spoke (yoda/darwin/gamma-guardian) despachar e com que pacote, a partir de `activation-policy.json` | [`agent-route.py`](agent-route.py) |
| Compilador do índice do brain | Compila `brain/` inteiro em índices por pasta, backlinks e diagnóstico, a partir do frontmatter de cada página | [`brain-index.py`](brain-index.py) |
| Emissor de memória do SessionStart | Monta os blocos de memória (recente/semanal/L3) injetados no início da sessão, em um spawn só | [`session-memory-emit.py`](session-memory-emit.py) |
| Roteador do brain | Índice invertido de termo → página do brain, para achar o que já existe sem abrir pasta | [`brain-route.py`](brain-route.py) |
| Migrador de workspace | Migra uma workspace `data/` (v0.1.11 e anteriores) para o layout `brain/` atual, sem sobrescrever | [`migrate-data-to-brain.py`](migrate-data-to-brain.py) |
| Verificador de caminhos | Confere que todo caminho citado entre crases em skills, agents, hooks e `CLAUDE.md` existe de verdade no disco | [`paths-check.py`](paths-check.py) |
| Verificador de leitores | Confere que todo arquivo de configuração do produto tem alguém que o leia — código ou prompt, nunca só citação | [`readers-check.py`](readers-check.py) |
| Roteador de skills | Índice invertido de termo → skill, bilíngue, para rotear a mensagem do dono sem injetar as 56 skills inteiras | [`skill-route.py`](skill-route.py) |

## Hooks (`.claude/hooks/`)

| Hook | Evento | O que faz | Pointer |
|---|---|---|---|
| Anúncio de despacho de agente | PreToolUse | Anuncia no chat, antes da chamada, quando um spoke vai ser despachado | [`../../../.claude/hooks/announce-agent-dispatch.sh`](../../../.claude/hooks/announce-agent-dispatch.sh) |
| Guard de isolamento entre casos | PreToolUse | Bloqueia escrita fora do caso ativo — a única barreira técnica entre clientes | [`../../../.claude/hooks/block-cross-case-writes.sh`](../../../.claude/hooks/block-cross-case-writes.sh) |
| Roteamento por intenção | UserPromptSubmit | Roteia cada mensagem do dono contra o índice de skills e de páginas do brain | [`../../../.claude/hooks/context-inject-userprompt.sh`](../../../.claude/hooks/context-inject-userprompt.sh) |
| Scaffold de primeira execução | SessionStart | Monta `brain/` na primeira sessão e detecta upgrade de versão instalada | [`../../../.claude/hooks/first-run-scaffold.sh`](../../../.claude/hooks/first-run-scaffold.sh) |
| Injeção de contexto da sessão | SessionStart | Injeta skills instaladas, caso ativo, brief do caso e blocos de memória | [`../../../.claude/hooks/session-start-memory-inject.sh`](../../../.claude/hooks/session-start-memory-inject.sh) |
| Gatilho automático de agentes de sistema | Stop | Verifica periodicamente se darwin/gamma-guardian precisam rodar | [`../../../.claude/hooks/session-stop-agent-check.sh`](../../../.claude/hooks/session-stop-agent-check.sh) |
| Recompilação do índice do brain | Stop | Recompila o índice do brain e avalia a saúde da estrutura ao fim de cada sessão | [`../../../.claude/hooks/session-stop-brain-index.sh`](../../../.claude/hooks/session-stop-brain-index.sh) |
| Marcador de dreaming | SessionEnd | Grava um marcador para a próxima sessão saber que um ciclo de consolidação de memória está pendente | [`../../../.claude/hooks/session-stop-dream.sh`](../../../.claude/hooks/session-stop-dream.sh) |

## Ver também

- [`../skills/INDEX.md`](../skills/INDEX.md) — índice das skills.
- [`../agents/catalog.json`](../agents/catalog.json) — catálogo dos agentes (hub/instâncias/spokes) e o grafo de quem pode despachar quem.
- [`../distribution.json`](../distribution.json) — mapa de tudo que viaja no pacote distribuído; todo `.sh` de hook e `.py` de tool listado acima precisa estar lá. Aqui quem empacota é `installers/zip/build-release.sh`, e o portão que recusa um arquivo não declarado é o teste de allowlist do harness, não o empacotador.

## Ainda não neste repositório

Uma linha saiu das tabelas acima porque o arquivo não existe aqui — um
ponteiro que não abre é pior que ausência, e este índice existe justamente para
ser seguido. Voltam com a mudança que as traz:

| O que falta | Chega com |
|---|---|
| `session-stop-eod-check.sh` — checagem de fechamento de dia | a mudança dos rituais do dono |

Quem portar reinsere a linha na tabela correspondente.
